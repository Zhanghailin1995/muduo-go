package memcached_server

import (
	"muduo"
	"muduo/pkg/util"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var _g_cas uint64

type slot struct {
	items map[string]*Item
	mu    sync.Mutex
}

type Options struct {
	engineCnt int
	port      int
}

func loadOptions(options ...Option) *Options {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	return opts
}

type Option func(opts *Options)

type Memcached struct {
	slots    []*slot
	slotn    int
	server   *muduo.TcpServer
	el       *muduo.Eventloop
	stime    time.Time
	sessions map[string]*Session
	mu       sync.Mutex
}

func WithEngineCnt(engineCnt int) Option {
	return func(opts *Options) {
		opts.engineCnt = engineCnt
	}
}

func WithPort(port int) Option {
	return func(opts *Options) {
		opts.port = port
	}
}

func NewMemcached(el *muduo.Eventloop, opts ...Option) *Memcached {
	options := loadOptions(opts...)
	m := &Memcached{
		slotn:    1024,
		slots:    make([]*slot, 1024),
		server:   muduo.NewTcpServer(el, "memcached", "tcp4://:"+strconv.Itoa(options.port), options.engineCnt),
		el:       el,
		sessions: make(map[string]*Session),
	}
	for i := 0; i < m.slotn; i++ {
		m.slots[i] = &slot{
			items: make(map[string]*Item),
		}
	}
	return m
}

func (m *Memcached) Start() {
	m.stime = time.Now()
	m.server.Start()
}

func (m *Memcached) setEngineCnt(engineCnt int) {
	m.server.SetEngineCnt(engineCnt)
}

func (m *Memcached) startTime() time.Time {
	return m.stime
}

func (m *Memcached) Store(item *Item, policy UpdatePolicy) (success bool, exist bool) {
	slot := m.slots[item._hash%uint64(m.slotn)]
	items := slot.items
	keyStr := string(item.Key())
	success = false
	slot.mu.Lock()
	defer slot.mu.Unlock()
	oldItem, exist := items[keyStr]
	if policy == Set {
		item.cas = atomic.AddUint64(&_g_cas, 1)
		items[keyStr] = item
		success = true
	} else {
		if policy == Add {
			if exist {
				return
			} else {
				item.cas = atomic.AddUint64(&_g_cas, 1)
				items[keyStr] = item
				success = true
			}
		} else if policy == Replace {
			if exist {
				item.cas = atomic.AddUint64(&_g_cas, 1)
				items[keyStr] = item
				success = true
			} else {
				return
			}
		} else if policy == Append || policy == Prepend {
			if exist {
				newLen := item.valuelen + oldItem.valuelen - 2
				newItem := NewItem(string(item.Key()), item.flags, item.exptime, newLen, atomic.AddUint64(&_g_cas, 1))
				if policy == Append {
					newItem.Append(oldItem.Value()[:oldItem.valuelen-2])
					newItem.Append(item.Value())
				} else {
					newItem.Append(item.Value()[:item.valuelen-2])
					newItem.Append(oldItem.Value())
				}
				util.Assert(newItem.keylen+newItem.valuelen-newItem.recvBytes == 0, "newItem.keylen+newItem.valuelen-newItem.recvBytes == 0")
				util.Assert(newItem.EndWithCRLF(), "newItem.EndWithCRLF()")
				items[keyStr] = newItem
				success = true
			} else {
				return
			}
		} else if policy == Cas {
			if exist && oldItem.cas == item.cas {
				item.cas = atomic.AddUint64(&_g_cas, 1)
				items[keyStr] = item
				success = true
			} else {
				return
			}
		} else {
			util.Assert(false, "invalid policy")
		}
	}
	return
}

func (m *Memcached) Get(key string) (item *Item, exist bool) {
	hash := hash(key)
	slot := m.slots[hash%uint64(m.slotn)]
	items := slot.items
	slot.mu.Lock()
	defer slot.mu.Unlock()
	item, exist = items[key]
	return
}

func (m *Memcached) Delete(key string) (success bool) {
	hash := hash(key)
	slot := m.slots[hash%uint64(m.slotn)]
	items := slot.items
	slot.mu.Lock()
	defer slot.mu.Unlock()
	_, exist := items[key]
	if exist {
		delete(items, key)
		success = true
	}
	return
}

func (m *Memcached) onConn(conn *muduo.TcpConn) {
	if conn.IsConnected() {
		sess := NewSession(m, conn)
		m.mu.Lock()
		defer m.mu.Unlock()
		util.Assert(m.sessions[conn.Name()] == nil, "m.sessions[conn.Name()] == nil")
		m.sessions[conn.Name()] = sess
	} else {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.sessions, conn.Name())
	}
}

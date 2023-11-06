package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

type TcpServer struct {
	el              *Eventloop
	name            string
	ac              *acceptor
	group           *EventloopEngineGroup
	onConn          func(*TcpConn)
	onMsg           func(*TcpConn, *Buffer, time.Time)
	onWriteComplete func(*TcpConn)
	started         bool
	nextConnId      uint64
	connMap         map[string]*TcpConn
	tcpNoDelay      int32
	keepAlive       int32
}

func NewTcpServer(el *Eventloop, name string, addr string, engineCnt int) *TcpServer {
	s := &TcpServer{
		el:         el,
		name:       name,
		ac:         newAcceptor(el, addr, nil),
		group:      NewEventloopEngineGroup(engineCnt, el),
		started:    false,
		nextConnId: 1,
		connMap:    make(map[string]*TcpConn),
		tcpNoDelay: 1,
		keepAlive:  1,
	}
	s.ac.cb = s.newConn // acceptor callback
	return s
}

func (s *TcpServer) SetTcpNoDelay(tcpNoDelay bool) {
	if tcpNoDelay {
		atomic.StoreInt32(&s.tcpNoDelay, 1)
	} else {
		atomic.StoreInt32(&s.tcpNoDelay, 0)
	}
}

func (s *TcpServer) Start() {
	if !s.started {
		s.started = true
		s.group.Start()
	}
	if !s.ac.listening {
		s.el.AsyncExecute(func() {
			s.ac.listen()
		})
	}
}

func (s *TcpServer) SetOnConn(cb func(*TcpConn)) {
	s.onConn = cb
}

func (s *TcpServer) SetOnMsg(cb func(*TcpConn, *Buffer, time.Time)) {
	s.onMsg = cb
}

func (s *TcpServer) SetOnWriteComplete(cb func(*TcpConn)) {
	s.onWriteComplete = cb
}

func (s *TcpServer) newConn(fd int, addr net.Addr) {
	connName := s.name + "-conn-" + strconv.Itoa(int(s.nextConnId))
	s.nextConnId++
	logging.Infof("new connection: fd=%d, addr=%s", fd, addr.String())
	// TODO get local addr
	//localAddr0, err := unix.Getsockname(fd)
	//if err != nil {
	//	logging.Errorf("getsockname error: %v", err)
	//	return
	//}
	//localAddr := SockaddrToTCPAddr(localAddr0)
	localAddr := s.ac.localAddr
	el := s.group.GetNextLoop()
	conn := NewTcpConn(el, connName, fd, localAddr, addr)
	if atomic.LoadInt32(&s.tcpNoDelay) == 1 {
		_ = conn.SetTcpNoDelay(true)
	}
	// 为什么要将conn放到map中呢？
	s.connMap[connName] = conn
	conn.SetOnConn(s.onConn)
	conn.SetOnMsg(s.onMsg)
	conn.SetOnWriteComplete(s.onWriteComplete)
	conn.setOnClose(s.removeConn)
	el.AsyncExecute(func() {
		conn.connectEstablished()
	})
}

func (s *TcpServer) removeConn(conn *TcpConn) {
	s.el.AsyncExecute(func() {
		s.removeConnInLoop(conn)
	})
}

func (s *TcpServer) removeConnInLoop(conn *TcpConn) {
	delete(s.connMap, conn.name)
	el := conn.el
	el.AsyncExecute(func() {
		conn.connectDestroyed()
		err := conn.so.close()
		if err != nil {
			logging.Errorf("close socket error: %v", err)
		}
	})
}

// SockaddrToTCPAddr converts a Sockaddr to a net.TCPAddr
// Returns nil if conversion fails.
func SockaddrToTCPAddr(sa unix.Sockaddr) *net.TCPAddr {
	ip, zone := SockaddrToIPAndZone(sa)
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return &net.TCPAddr{IP: ip, Port: sa.Port}
	case *unix.SockaddrInet6:
		return &net.TCPAddr{IP: ip, Port: sa.Port, Zone: zone}
	}
	return nil
}

// SockaddrToIPAndZone converts a Sockaddr to a net.IP (with optional IPv6 Zone)
// Returns nil if conversion fails.
func SockaddrToIPAndZone(sa unix.Sockaddr) (net.IP, string) {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		ip := make([]byte, 16)
		// V4InV6Prefix
		ip[10] = 0xff
		ip[11] = 0xff
		copy(ip[12:16], sa.Addr[:])
		return ip, ""

	case *unix.SockaddrInet6:
		ip := make([]byte, 16)
		copy(ip, sa.Addr[:])
		return ip, IP6ZoneToString(int(sa.ZoneId))
	}
	return nil, ""
}

// IP6ZoneToString converts an IP6 Zone unix int to a net string
// returns "" if zone is 0
func IP6ZoneToString(zone int) string {
	if zone == 0 {
		return ""
	}
	if ifi, err := net.InterfaceByIndex(zone); err == nil {
		return ifi.Name
	}
	return itod(uint(zone))
}

// Convert i to decimal string.
func itod(i uint) string {
	if i == 0 {
		return "0"
	}

	// Assemble decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; i > 0; i /= 10 {
		bp--
		b[bp] = byte(i%10) + '0'
	}

	return string(b[bp:])
}

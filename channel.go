package muduo

import (
	"container/list"
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"time"
)

type EventCallback func()
type ReadCallback func(ts time.Time)

const (
	eventRead  = unix.POLLIN | unix.POLLPRI
	eventWrite = unix.POLLOUT
	eventNone  = 0
)

type Channel struct {
	ele           *list.Element
	el            *Eventloop
	fd            int
	events        uint32
	revents       uint32
	index         int
	readCallback  ReadCallback
	writeCallback EventCallback
	errorCallback EventCallback
	closeCallback EventCallback
	evtHandling   bool
}

func NewChannel(el *Eventloop, fd int) *Channel {
	return &Channel{
		ele:         nil,
		el:          el,
		fd:          fd,
		events:      eventNone,
		revents:     0,
		index:       channelNew,
		evtHandling: false,
	}
}

func (c *Channel) setReadCallback(cb ReadCallback) {
	c.readCallback = cb
}

func (c *Channel) setWriteCallback(cb EventCallback) {
	c.writeCallback = cb
}

func (c *Channel) setErrorCallback(cb EventCallback) {
	c.errorCallback = cb
}

func (c *Channel) setCloseCallback(cb EventCallback) {
	c.closeCallback = cb
}

func (c *Channel) enableReading() {
	c.events |= eventRead
	c.update()
}

func (c *Channel) disableAll() {
	c.events = eventNone
	c.update()
}

func (c *Channel) disableWriting() {
	c.events &= ^uint32(eventWrite)
	c.update()
}

func (c *Channel) enableWriting() {
	c.events |= eventWrite
	c.update()
}

func (c *Channel) isWriting() bool {
	return c.events&eventWrite != 0
}

func (c *Channel) IsNoneEvent() bool {
	return c.events == eventNone
}

func (c *Channel) update() {
	c.el.updateChannel(c)
}

func (c *Channel) handleEvent(ts time.Time) {
	c.evtHandling = true
	logging.Debugf("Channel::handleEvent() %s", events2String(c.fd, c.revents))
	if c.revents&unix.POLLNVAL != 0 {
		logging.Warnf("Channel::handleEvent() POLLNVAL")
	}
	if c.revents&unix.POLLHUP != 0 && c.revents&unix.POLLIN == 0 {
		logging.Warnf("Channel::handleEvent() POLLHUP")
		if c.closeCallback != nil {
			c.closeCallback()
		}
	}
	if c.revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
		if c.errorCallback != nil {
			c.errorCallback()
		}
	}
	if c.revents&(unix.POLLIN|unix.POLLPRI|unix.POLLRDHUP) != 0 {
		if c.readCallback != nil {
			c.readCallback(ts)
		}
	}
	if c.revents&unix.POLLOUT != 0 {
		if c.writeCallback != nil {
			c.writeCallback()
		}
	}
	c.evtHandling = false
}

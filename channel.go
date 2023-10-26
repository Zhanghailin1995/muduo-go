package muduo

import (
	"container/list"
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
)

type EventCallback func()

const (
	eventRead  = unix.POLLIN | unix.POLLPRI
	eventWrite = unix.POLLOUT
	eventNone  = 0
)

type Channel struct {
	ele           *list.Element
	el            *eventloop
	fd            int
	events        uint32
	revents       uint32
	index         int
	readCallback  EventCallback
	writeCallback EventCallback
	errorCallback EventCallback
}

func NewChannel(el *eventloop, fd int) *Channel {
	return &Channel{
		ele:     nil,
		el:      el,
		fd:      fd,
		events:  eventNone,
		revents: 0,
		index:   -1,
	}
}

func (c *Channel) enableReading() {
	c.events |= eventRead
	c.update()
}

func (c *Channel) IsNoneEvent() bool {
	return c.events == eventNone
}

func (c *Channel) update() {
	c.el.updateChannel(c)
}

func (c *Channel) handleEvent() {
	if c.revents&unix.POLLNVAL != 0 {
		logging.Warnf("Channel::handleEvent() POLLNVAL")
	}
	if c.revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
		if c.errorCallback != nil {
			c.errorCallback()
		}
	}
	if c.revents&(unix.POLLIN|unix.POLLPRI|unix.POLLRDHUP) != 0 {
		if c.readCallback != nil {
			c.readCallback()
		}
	}
	if c.revents&unix.POLLOUT != 0 {
		if c.writeCallback != nil {
			c.writeCallback()
		}
	}
}

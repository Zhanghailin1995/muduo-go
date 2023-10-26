package muduo

import (
	"container/list"
	"muduo/pkg/logging"
	"sync/atomic"
)

const (
	pollTimeoutMills = 10000
)

type eventloop struct {
	looping        int32
	quit           int32
	activeChannels *list.List
	poller         *Poller
}

func NewEventloop() *eventloop {
	el := &eventloop{
		looping:        0,
		quit:           0,
		activeChannels: list.New(),
	}
	el.poller, _ = NewPoller(el)
	return el
}

func (el *eventloop) loop() {
	atomic.StoreInt32(&el.looping, 1)
	atomic.StoreInt32(&el.quit, 0)
	logging.Infof("eventloop start looping")
	for {
		if atomic.LoadInt32(&el.quit) != 0 {
			break
		}
		el.activeChannels.Init()
		el.poller.poll(pollTimeoutMills)
		for e := el.activeChannels.Front(); e != nil; e = e.Next() {
			channel := e.Value.(*Channel)
			channel.handleEvent()
		}
	}

	logging.Infof("eventloop stop looping")

	atomic.StoreInt32(&el.looping, 0)
}

func (el *eventloop) stop() {
	atomic.StoreInt32(&el.quit, 1)
}

func (el *eventloop) updateChannel(channel *Channel) {
	el.poller.updateChannel(channel)
}

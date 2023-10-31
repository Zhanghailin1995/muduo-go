package muduo

import (
	"container/list"
	"muduo/pkg/logging"
	"sync/atomic"
	"time"
)

const (
	pollTimeoutMills = 10000
)

type Task func()

type TimeoutCallback func()

type Eventloop struct {
	looping        int32
	quit           int32
	activeChannels *list.List
	poller         *Poller
	tq             *timerQueue
}

func NewEventloop() *Eventloop {
	el := &Eventloop{
		looping:        0,
		quit:           0,
		activeChannels: list.New(),
	}
	el.poller, _ = newPoller(el)
	el.tq = newTimerQueue(el)
	return el
}

func (el *Eventloop) Schedule(callback TimeoutCallback, t time.Time) {
	el.tq.addTask(callback, t, 0)
}

func (el *Eventloop) ScheduleDelay(callback TimeoutCallback, d time.Duration) {
	el.tq.addTask(callback, time.Now().Add(d), 0)
}

func (el *Eventloop) ScheduleAtFixRate(callback TimeoutCallback, interval time.Duration) {
	t := time.Now()
	el.tq.addTask(callback, t.Add(interval), interval)
}

func (el *Eventloop) loop() {
	atomic.StoreInt32(&el.looping, 1)
	atomic.StoreInt32(&el.quit, 0)
	logging.Infof("Eventloop start looping")
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

	logging.Infof("Eventloop stop looping")

	atomic.StoreInt32(&el.looping, 0)
}

func (el *Eventloop) stop() {
	atomic.StoreInt32(&el.quit, 1)
}

func (el *Eventloop) updateChannel(channel *Channel) {
	el.poller.updateChannel(channel)
}

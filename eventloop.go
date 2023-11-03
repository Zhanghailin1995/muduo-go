package muduo

import (
	"container/list"
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	pollTimeoutMills = 10000
)

type Task func()

type TimeoutCallback func()

type Eventloop struct {
	id                  string
	looping             int32
	quit                int32
	activeChannels      *list.List
	poller              *Poller
	tq                  *timerQueue
	pendingTasks        *list.List
	taskMutex           sync.Mutex
	evtFd               int
	wakeupChannel       *Channel
	runningPendingTasks bool
}

func NewEventloop(id string) *Eventloop {
	el := &Eventloop{
		id:                  id,
		looping:             0,
		quit:                0,
		activeChannels:      list.New(),
		pendingTasks:        list.New(),
		runningPendingTasks: false,
	}
	el.poller, _ = newPoller(el)
	el.tq = newTimerQueue(el)
	el.evtFd = createEventFd()
	wakeupChannel := NewChannel(el, el.evtFd)
	el.wakeupChannel = wakeupChannel
	wakeupChannel.setReadCallback(el.handleRead)
	wakeupChannel.enableReading()
	return el
}

func (el *Eventloop) handleRead(_ time.Time) {
	var one uint64
	_, _ = unix.Read(el.evtFd, (*(*[8]byte)(unsafe.Pointer(&one)))[:])
}

func createEventFd() int {
	evtFd, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		panic(err)
	}
	return evtFd
}

//func (el *Eventloop) Execute(task Task) {
//	task()
//}

func (el *Eventloop) AsyncExecute(task Task) {
	logging.Infof("eventloop[%s] async execute task", el.id)
	el.taskMutex.Lock()
	el.pendingTasks.PushBack(task)
	el.taskMutex.Unlock()

	el.Wakeup()
}

func (el *Eventloop) Wakeup() {
	var one uint64 = 1
	_, _ = unix.Write(el.evtFd, (*(*[8]byte)(unsafe.Pointer(&one)))[:])
}

func (el *Eventloop) runPendingTasks() {
	pt := el.pendingTasks
	el.runningPendingTasks = true
	el.taskMutex.Lock()
	el.pendingTasks = list.New()
	el.taskMutex.Unlock()
	for e := pt.Front(); e != nil; e = e.Next() {
		task := e.Value.(Task)
		task()
	}
	el.runningPendingTasks = false
}

func (el *Eventloop) Schedule(callback TimeoutCallback, t time.Time) {
	el.tq.addTask(callback, t, 0)
}

func (el *Eventloop) AsyncSchedule(callback TimeoutCallback, d time.Duration) {
	el.AsyncExecute(func() {
		el.Schedule(callback, time.Now().Add(d))
	})
}

func (el *Eventloop) ScheduleDelay(callback TimeoutCallback, d time.Duration) {
	el.tq.addTask(callback, time.Now().Add(d), 0)
}

func (el *Eventloop) AsyncScheduleDelay(callback TimeoutCallback, d time.Duration) {
	el.AsyncExecute(func() {
		el.ScheduleDelay(callback, d)
	})
}

func (el *Eventloop) ScheduleAtFixRate(callback TimeoutCallback, interval time.Duration) {
	t := time.Now()
	el.tq.addTask(callback, t.Add(interval), interval)
}

func (el *Eventloop) AsyncScheduleAtFixRate(callback TimeoutCallback, interval time.Duration) {
	el.AsyncExecute(func() {
		el.ScheduleAtFixRate(callback, interval)
	})
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
		retTs := el.poller.poll(pollTimeoutMills)
		for e := el.activeChannels.Front(); e != nil; e = e.Next() {
			channel := e.Value.(*Channel)
			logging.Debugf("Eventloop[%s] handle event", el.id)
			channel.handleEvent(retTs)
		}
		el.runPendingTasks()
	}

	logging.Infof("Eventloop Stop looping")

	atomic.StoreInt32(&el.looping, 0)
	el.destroy()
}

func (el *Eventloop) destroy() {
	_ = unix.Close(el.evtFd)
}

func (el *Eventloop) Stop() {
	atomic.StoreInt32(&el.quit, 1)
}

func (el *Eventloop) AsyncStop() {
	atomic.StoreInt32(&el.quit, 1)
	el.Wakeup()
}

func (el *Eventloop) updateChannel(channel *Channel) {
	el.poller.updateChannel(channel)
}

func (el *Eventloop) removeChannel(channel *Channel) {
	el.poller.removeChannel(channel)
}

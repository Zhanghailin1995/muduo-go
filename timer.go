package muduo

import (
	"container/heap"
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"muduo/pkg/util"
	"time"
	"unsafe"
)

var (
	invalidTime = time.Unix(0, 0)
)

type timerTask struct {
	expire   time.Time
	interval time.Duration
	repeat   bool
	cb       func()
}

func (tt *timerTask) run() {
	tt.cb()
}

func (tt *timerTask) restart(t time.Time) {
	if tt.repeat {
		tt.expire = t.Add(tt.interval)
	} else {
		tt.expire = time.Unix(0, 0)
	}
}

type timerQueue struct {
	el             *Eventloop
	timerFd        int
	timerFdChannel *Channel
	tasks          *timerTaskHeap
}

func newTimerQueue(el *Eventloop) *timerQueue {
	timerFd, err := unix.TimerfdCreate(unix.CLOCK_MONOTONIC, unix.TFD_NONBLOCK|unix.TFD_CLOEXEC)
	if err != nil {
		panic(err)
	}
	timerFdChannel := NewChannel(el, timerFd)
	tq := &timerQueue{
		el:             el,
		timerFd:        timerFd,
		timerFdChannel: timerFdChannel,
		tasks: &timerTaskHeap{
			tasks: make([]*timerTask, 0),
		},
	}
	timerFdChannel.setReadCallback(tq.handleRead)
	timerFdChannel.enableReading()
	return tq
}

func (tq *timerQueue) addTask(cb func(), t time.Time, interval time.Duration) *timerTask {
	tt := &timerTask{
		expire:   t,
		cb:       cb,
		interval: interval,
		repeat:   interval > 0,
	}
	earliestChanged := tq.insert(tt)
	if earliestChanged {
		logging.Debugf("timerQueue::addTask() earliestChanged")
		resetTimerFd(tq.timerFd, t)
	}
	return tt
}

func (tq *timerQueue) handleRead(ts time.Time) {
	logging.Debugf("timerQueue::handleRead()")
	var exp uint64
	_, err := unix.Read(tq.timerFd, (*(*[8]byte)(unsafe.Pointer(&exp)))[:])
	if err != nil {
		logging.Errorf("timerQueue::handleRead() %v", err)
	}
	now := time.Now()
	expiredTask := tq.getExpired(now)
	for _, v := range expiredTask {
		v.run()
	}
	tq.reset(expiredTask, now)
}

func (tq *timerQueue) shutdown() {
	_ = unix.Close(tq.timerFd)
}

func (tq *timerQueue) reset(expired []*timerTask, t time.Time) {
	for _, v := range expired {
		if v.repeat {
			v.restart(t)
			tq.insert(v)
		}
	}
	if tq.tasks.Len() > 0 {
		nextExpire := tq.tasks.Top().expire
		if nextExpire.After(invalidTime) {
			// reset timerfd
			resetTimerFd(tq.timerFd, nextExpire)
		}
	}
}

func resetTimerFd(timerFd int, t time.Time) {
	var its unix.ItimerSpec
	var oldTs unix.ItimerSpec
	// its.Value = unix.NsecToTimespec(t.UnixNano())
	duration := t.Sub(time.Now())
	if duration.Microseconds() < 100 {
		duration = 100 * time.Microsecond
	}
	its.Value = unix.NsecToTimespec(duration.Nanoseconds())
	err := unix.TimerfdSettime(timerFd, 0, &its, &oldTs)
	if err != nil {
		logging.Errorf("resetTimerFd() %v", err)
	}
}

func (tq *timerQueue) insert(task *timerTask) bool {
	earliestChanged := false
	expire := task.expire
	if tq.tasks.Len() == 0 || expire.Before(tq.tasks.Top().expire) {
		earliestChanged = true
	}
	heap.Push(tq.tasks, task)
	return earliestChanged

}

func (tq *timerQueue) getExpired(t time.Time) []*timerTask {
	return tq.tasks.getAndRemoveExpired(t)
}

type timerTaskHeap struct {
	tasks []*timerTask
}

func (h *timerTaskHeap) Len() int {
	return len(h.tasks)
}

func (h *timerTaskHeap) Less(i, j int) bool {
	if h.tasks[i].expire.Before(h.tasks[j].expire) {
		return true
	} else if h.tasks[i].expire.Equal(h.tasks[j].expire) {
		return i < j
	} else {
		return false
	}
}

func (h *timerTaskHeap) Swap(i, j int) {
	h.tasks[i], h.tasks[j] = h.tasks[j], h.tasks[i]
}

func (h *timerTaskHeap) Push(x interface{}) {
	h.tasks = append(h.tasks, x.(*timerTask))
}

func (h *timerTaskHeap) Pop() interface{} {
	old := h.tasks
	n := len(old)
	x := old[n-1]
	old[n-1] = nil // avoid memory leak
	h.tasks = old[0 : n-1]
	return x
}

func (h *timerTaskHeap) Top() *timerTask {
	return h.tasks[0]
}

func (h *timerTaskHeap) getAndRemoveExpired(t time.Time) []*timerTask {
	var ret []*timerTask
	for {
		if h.Len() == 0 {
			break
		}
		pop := h.Top()
		if pop.expire.Before(t) || pop.expire.Equal(t) {
			tmp := heap.Pop(h)
			util.Assert(tmp == pop, "t == pop")
			ret = append(ret, pop)
		} else {
			break
		}
	}
	return ret
}

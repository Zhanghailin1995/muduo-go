package muduo

import (
	"container/heap"
	"math/rand"
	"testing"
	"time"
)

func TestLowerBound(t *testing.T) {
	timers := &timerTaskHeap{
		tasks: make([]*TimerTask, 0),
	}
	for i := 0; i < 10; i++ {

		heap.Push(timers, &TimerTask{
			expire:   time.Unix(int64(rand.Intn(100)), 0),
			interval: time.Second,
			repeat:   true,
			cb:       func() {},
		})
	}

	for _, v := range timers.tasks {
		t.Logf("%d", v.expire.Unix())
	}

	t.Logf("----------------------")

	ret := timers.getAndRemoveExpired(time.Unix(30, 0))
	for _, v := range ret {
		t.Logf("%d", v.expire.Unix())
	}

}

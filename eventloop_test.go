package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"testing"
	"time"
	"unsafe"
)

func TestEventloop_Loop(t *testing.T) {
	el := &Eventloop{}
	el.loop()
}

func TestEventloop_Timefd(t *testing.T) {
	el := NewEventloop()
	timerFd, err := unix.TimerfdCreate(unix.CLOCK_MONOTONIC, unix.TFD_NONBLOCK|unix.TFD_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	ch := NewChannel(el, timerFd)
	ch.readCallback = func() {
		t.Log("timerfd readCallback")
		// read timefd
		var exp uint64
		_, err := unix.Read(timerFd, (*(*[8]byte)(unsafe.Pointer(&exp)))[:])
		if err != nil {
			t.Fatal(err)
		}
	}
	ch.enableReading()
	howlong := unix.ItimerSpec{}
	howlong.Value.Sec = 5
	unix.TimerfdSettime(timerFd, 0, &howlong, nil)
	el.loop()
	unix.Close(timerFd)
}

func TestEventloop_Schedule(t *testing.T) {
	el := NewEventloop()
	when := time.Now().Add(time.Second * 2)
	el.Schedule(func() {
		logging.Infof("hello world")
		el.stop()
	}, when)
	el.loop()

	logging.Infof("eventloop stopped")
}

func TestEventloop_ScheduleDelay(t *testing.T) {
	el := NewEventloop()
	el.ScheduleDelay(func() {
		logging.Infof("hello world")
		el.stop()
	}, time.Second*2)
	el.loop()

	logging.Infof("eventloop stopped")
}

func TestEventloop_ScheduleAtFixRate(t *testing.T) {
	var count int
	el := NewEventloop()
	el.ScheduleAtFixRate(func() {
		count++
		logging.Infof("hello world: %d", count)
		if count == 5 {
			el.stop()
		}
	}, time.Second*2)
	el.loop()

	logging.Infof("eventloop stopped")
}

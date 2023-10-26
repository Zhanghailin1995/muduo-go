package muduo

import (
	"golang.org/x/sys/unix"
	"testing"
	"unsafe"
)

func TestEventloop_Loop(t *testing.T) {
	el := &eventloop{}
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

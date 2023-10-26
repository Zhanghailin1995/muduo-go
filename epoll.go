package muduo

import (
	"golang.org/x/sys/unix"
	"unsafe"
)

type epollevent struct {
	events uint32
	data   [8]byte // unaligned uintptr
}

func epollWait(epfd int, events []epollevent, msec int) (int, error) {
	var ep unsafe.Pointer
	if len(events) > 0 {
		ep = unsafe.Pointer(&events[0])
	} else {
		ep = unsafe.Pointer(&zero)
	}
	var (
		np    uintptr
		errno unix.Errno
	)
	if msec == 0 { // non-block system call, use RawSyscall6 to avoid getting preempted by runtime
		np, _, errno = unix.RawSyscall6(unix.SYS_EPOLL_WAIT, uintptr(epfd), uintptr(ep), uintptr(len(events)), 0, 0, 0)
	} else {
		np, _, errno = unix.Syscall6(unix.SYS_EPOLL_WAIT, uintptr(epfd), uintptr(ep), uintptr(len(events)), uintptr(msec), 0, 0)
	}
	if errno != 0 {
		return int(np), errnoErr(errno)
	}
	return int(np), nil
}

func epollCtl(epfd int, op int, fd int, event *epollevent) error {
	_, _, errno := unix.RawSyscall6(unix.SYS_EPOLL_CTL, uintptr(epfd), uintptr(op), uintptr(fd), uintptr(unsafe.Pointer(event)), 0, 0)
	if errno != 0 {
		return errnoErr(errno)
	}
	return nil
}

// Do the interface allocations only once for common
// Errno values.
var (
	errEAGAIN error = unix.EAGAIN
	errEINVAL error = unix.EINVAL
	errENOENT error = unix.ENOENT
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e unix.Errno) error {
	switch e {
	case unix.EAGAIN:
		return errEAGAIN
	case unix.EINVAL:
		return errEINVAL
	case unix.ENOENT:
		return errENOENT
	}
	return e
}

var zero uintptr

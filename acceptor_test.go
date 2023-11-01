package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"net"
	"testing"
)

func TestAcceptor(t *testing.T) {
	el := NewEventloop()
	ac := newAcceptor(el, "tcp4://:9981", nil)
	ac.cb = func(fd int, addr net.Addr) {
		logging.Debugf("new connection: fd=%d, addr=%s", fd, addr.String())
		_, _ = unix.Write(fd, []byte("how are you?\n"))
	}
	ac.listen()
	el.loop()
}

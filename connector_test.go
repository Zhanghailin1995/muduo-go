package muduo

import (
	"muduo/pkg/logging"
	"testing"
)

func TestNewConnector(t *testing.T) {
	el := NewEventloop("test")
	connector, err := NewConnector(el, "tcp4://192.168.1.57:9681", nil)
	if err != nil {
		t.Error(err)
	}
	connector.cb = func(fd int) {
		logging.Infof(">>>>>>>>>>>>>>>>>>>>fd: %d", fd)
	}
	connector.Start()
	el.Loop()
}

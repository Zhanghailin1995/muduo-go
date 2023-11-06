package muduo

import (
	"muduo/pkg/logging"
	"testing"
	"time"
)

func TestNewTcpClient(t *testing.T) {
	el := NewEventloop("test")
	client, err := NewTcpClient(el, "tcp4://192.168.1.57:9681")
	client.SetRetry(true)
	if err != nil {
		t.Error(err)
	}
	client.SetOnConn(func(conn *TcpConn) {
		if conn.state == Connected {
			logging.Infof("connection established: %s, addr=%s", conn.name, conn.peerAddr.String())
		} else {
			logging.Infof("connection closed: %s, addr=%s", conn.name, conn.peerAddr.String())
		}
	})
	client.SetOnMsg(func(conn *TcpConn, buffer *Buffer, t time.Time) {
		logging.Infof("message received: fd=%d, addr=%s, msg=%s", conn.so.fd, conn.peerAddr.String(), string(buffer.Next(-1)))
	})
	client.Connect()
	el.Loop()
}

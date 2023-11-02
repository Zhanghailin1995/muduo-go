package muduo

import (
	"muduo/pkg/logging"
	"testing"
	"time"
)

func TestNewTcpServer(t *testing.T) {
	el := NewEventloop()
	svr := NewTcpServer(el, "hello", "tcp4://:4589")
	svr.SetOnConn(func(conn *TcpConn) {
		if conn.state == Connected {
			logging.Infof("connection established: fd=%d, addr=%s", conn.so.fd, conn.peerAddr.String())
		} else {
			logging.Infof("connection closed: fd=%d, addr=%s", conn.so.fd, conn.peerAddr.String())
		}
	})

	svr.SetOnMsg(func(conn *TcpConn, buffer *Buffer, t time.Time) {
		logging.Infof("message received: fd=%d, addr=%s, msg=%s", conn.so.fd, conn.peerAddr.String(), string(buffer.Next(-1)))
	})

	svr.Start()
	el.loop()
}

func TestTcpServer_Write(t *testing.T) {
	el := NewEventloop()
	svr := NewTcpServer(el, "hello", "tcp4://:4589")
	svr.SetOnConn(func(conn *TcpConn) {
		if conn.state == Connected {
			logging.Infof("connection established: fd=%d, addr=%s", conn.so.fd, conn.peerAddr.String())
			_, _ = conn.Write([]byte("hello world"))
		} else {
			logging.Infof("connection closed: fd=%d, addr=%s", conn.so.fd, conn.peerAddr.String())
		}
	})

	svr.SetOnMsg(func(conn *TcpConn, buffer *Buffer, t time.Time) {
		data := buffer.Next(-1)
		logging.Infof("message received: fd=%d, addr=%s, msg=%s", conn.so.fd, conn.peerAddr.String(), string(data))
		_, _ = conn.Write(data)
	})

	svr.Start()
	el.loop()
}

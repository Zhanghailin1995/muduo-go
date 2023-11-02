package muduo

import (
	"muduo/pkg/logging"
	"net"
	"time"
)

type ConnState int

const (
	Connecting ConnState = iota
	Connected
	Disconnected
)

type TcpConn struct {
	el        *Eventloop
	name      string
	so        *socket
	ch        *Channel
	localAddr net.Addr
	peerAddr  net.Addr
	state     ConnState
	onConn    func(*TcpConn)
	onMsg     func(*TcpConn, *Buffer, time.Time)
	onClose   func(*TcpConn)
	inbound   *Buffer
}

func NewTcpConn(el *Eventloop, name string, fd int, localAddr, peerAddr net.Addr) *TcpConn {
	conn := &TcpConn{
		el:        el,
		name:      name,
		state:     Connecting,
		so:        &socket{fd: fd},
		ch:        NewChannel(el, fd),
		localAddr: localAddr,
		peerAddr:  peerAddr,
		inbound:   NewBuffer(),
	}
	logging.Debugf("new connection: fd=%d, addr=%s", fd, peerAddr.String())
	conn.ch.setReadCallback(conn.handleRead)
	return conn
}

func (c *TcpConn) SetOnConn(cb func(*TcpConn)) {
	c.onConn = cb
}

func (c *TcpConn) SetOnMsg(cb func(*TcpConn, *Buffer, time.Time)) {
	c.onMsg = cb
}

func (c *TcpConn) SetOnClose(cb func(*TcpConn)) {
	c.onClose = cb
}

func (c *TcpConn) connectEstablished() {
	c.state = Connected
	c.ch.enableReading()
	if c.onConn != nil {
		c.onConn(c)
	}
}

func (c *TcpConn) connectDestroyed() {
	c.state = Disconnected
	c.ch.disableAll()
	c.onConn(c)
	c.el.removeChannel(c.ch)
}

func (c *TcpConn) handleRead(ts time.Time) {
	n, err := c.inbound.ReadFd(c.ch.fd)
	if err != nil {
		logging.Errorf("read error: %s", err.Error())
		c.handleError(err)
		return
	}
	if n > 0 {
		if c.onMsg != nil {
			c.onMsg(c, c.inbound, ts)
		}
	} else {
		c.handleClose()
	}
}

func (c *TcpConn) handleWrite() {

}

func (c *TcpConn) handleClose() {
	logging.Debugf("connection closed: fd=%d, addr=%s", c.so.fd, c.peerAddr.String())
	c.ch.disableAll()
	c.onClose(c)
}

func (c *TcpConn) handleError(err error) {
	logging.Debugf("connection error: fd=%d, addr=%s", c.so.fd, c.peerAddr.String())
}

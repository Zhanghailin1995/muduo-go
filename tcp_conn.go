package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/errors"
	"muduo/pkg/logging"
	"muduo/pkg/util"
	"net"
	"sync/atomic"
	"time"
	"unsafe"
)

type ConnState int32

type AsyncCallback func(c *TcpConn, err error) error

const (
	Connecting ConnState = iota
	Connected
	Disconnecting
	Disconnected
)

type TcpConn struct {
	el              *Eventloop
	name            string
	so              *socket
	ch              *Channel
	localAddr       net.Addr
	peerAddr        net.Addr
	state           ConnState
	onConn          func(*TcpConn)
	onMsg           func(*TcpConn, *Buffer, time.Time)
	onClose         func(*TcpConn)
	onWriteComplete func(*TcpConn)
	inbound         *Buffer
	outbound        *Buffer
	ctx             interface{}
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
		outbound:  NewBuffer(),
	}
	logging.Debugf("new connection: fd=%d, addr=%s", fd, peerAddr.String())
	conn.ch.setReadCallback(conn.handleRead)
	return conn
}

func (c *TcpConn) Eventloop() *Eventloop {
	return c.el
}

func (c *TcpConn) SetOnConn(cb func(*TcpConn)) {
	c.onConn = cb
}

func (c *TcpConn) SetOnMsg(cb func(*TcpConn, *Buffer, time.Time)) {
	c.onMsg = cb
}

func (c *TcpConn) setOnClose(cb func(*TcpConn)) {
	c.onClose = cb
}

func (c *TcpConn) SetOnWriteComplete(cb func(*TcpConn)) {
	c.onWriteComplete = cb
}

func (c *TcpConn) GetConnState() ConnState {
	return c.state
}

func (c *TcpConn) GetLocalAddr() net.Addr {
	return c.localAddr
}

func (c *TcpConn) GetPeerAddr() net.Addr {
	return c.peerAddr
}

func (c *TcpConn) GetInboundBuffer() *Buffer {
	return c.inbound
}

func (c *TcpConn) GetOutboundBuffer() *Buffer {
	return c.outbound
}

func (c *TcpConn) IsConnected() bool {
	return c.state == Connected
}

func (c *TcpConn) IsDisconnected() bool {
	return c.state == Disconnected
}

func (c *TcpConn) SetContext(ctx interface{}) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.ctx)), unsafe.Pointer(&ctx))
}

func (c *TcpConn) GetContext() interface{} {
	return *(*interface{})(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&c.ctx))))
}

func (c *TcpConn) Write(buf []byte) (int, error) {
	if c.state == Connected {
		if len(buf) == 0 {
			return 0, nil
		}
		var sent int
		// if no data in outbound buffer, try writing directly
		if !c.ch.isWriting() && c.outbound.ReadableBytes() == 0 {
			n, err := unix.Write(c.ch.fd, buf)
			if err != nil && err != unix.EWOULDBLOCK {
				logging.Errorf("write error: %v", err)
				return sent, err
			}
			sent = n
			if sent < len(buf) {
				logging.Debugf("write partial data: %d/%d", n, len(buf))
			} else {
				if c.onWriteComplete != nil {
					c.el.AsyncExecute(func() {
						c.onWriteComplete(c)
					})
				}
			}
		}
		if sent < len(buf) {
			_, _ = c.outbound.Write(buf[sent:])
			if !c.ch.isWriting() {
				c.ch.enableWriting()
			}
		}
		return sent, nil
	} else {
		return 0, errors.ErrConnNotOpened
	}
}

func (c *TcpConn) AsyncWrite(buf []byte, cb AsyncCallback) error {
	if c.state == Connected {
		c.el.AsyncExecute(func() {
			var err error
			_, err = c.Write(buf)
			if cb != nil {
				err = cb(c, err)
			}
			if err != nil {
				logging.Errorf("async write error: %v", err)
				c.handleError(err)
			}
		})
		return nil
	} else {
		return errors.ErrConnNotOpened
	}
}

func (c *TcpConn) ShutdownWrite() {
	if c.state == Connected {
		c.state = Disconnecting
		c.el.AsyncExecute(c.shutdownWrite)
	}
}

func (c *TcpConn) SetTcpNoDelay(enable bool) error {
	return c.so.setTcpNoDelay(enable)
}

func (c *TcpConn) shutdownWrite() {
	if !c.ch.isWriting() {
		err := unix.Shutdown(c.ch.fd, unix.SHUT_WR)
		if err != nil {
			logging.Errorf("shutdown error: %v", err)
		}
	}
}

func (c *TcpConn) connectEstablished() {
	util.Assert(c.state == Connecting, "state should be connecting")
	c.state = Connected
	c.ch.enableReading()
	if c.onConn != nil {
		c.onConn(c)
	}
}

func (c *TcpConn) connectDestroyed() {
	util.Assert(c.state == Connected || c.state == Disconnecting, "state should be connected or disconnecting")
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
	logging.Debugf("handle write: %d", c.outbound.ReadableBytes())
	if c.ch.isWriting() {
		data := c.outbound.Peek()
		n, err := unix.Write(c.ch.fd, data)
		if err != nil && err != unix.EWOULDBLOCK {
			logging.Errorf("write error: %v", err)
			c.handleError(err)
			return
		}
		c.outbound.Advance(n)
		if c.outbound.ReadableBytes() == 0 {
			c.ch.disableWriting()
			if c.onWriteComplete != nil {
				c.el.AsyncExecute(func() {
					c.onWriteComplete(c)
				})
			}
			if c.state == Disconnecting {
				c.shutdownWrite()
			}
		} else {
			logging.Debugf("more data to write: %d", c.outbound.ReadableBytes())
		}
	} else {
		logging.Warn("connection is down, no more writing")
	}
}

func (c *TcpConn) handleClose() {
	logging.Debugf("connection closed: fd=%d, addr=%s", c.so.fd, c.peerAddr.String())
	c.ch.disableAll()
	c.onClose(c)
}

func (c *TcpConn) handleError(err error) {
	// TODO onError
	logging.Errorf("connection error: %s, addr=%s, err: %v", c.name, c.peerAddr.String(), err)
}

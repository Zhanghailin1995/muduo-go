package muduo

import (
	"golang.org/x/sys/unix"
	"math"
	"muduo/pkg/logging"
	"time"
)

type ConnectState int

const (
	connectorDisconnected ConnectState = iota
	connectorConnecting
	connectorConnected
)

type Connector struct {
	el          *Eventloop
	svrAddr     string
	unixSvrAddr unix.Sockaddr
	connect     bool
	state       ConnectState
	ch          *Channel
	cb          func(int)
	retryDelay  time.Duration
	tt          *TimerTask
}

func NewConnector(el *Eventloop, svrAddr string, cb func(int)) (*Connector, error) {
	network, addr := parseProtoAddr(svrAddr)
	sa, _, _, _, err := GetTCPSockAddr(network, addr)
	if err != nil {
		return nil, err
	}
	return &Connector{
		el:          el,
		svrAddr:     svrAddr,
		unixSvrAddr: sa,
		connect:     false,
		state:       connectorDisconnected,
		ch:          nil,
		cb:          cb,
		retryDelay:  500 * time.Millisecond,
		tt:          nil,
	}, nil
}

func (c *Connector) Start() {
	c.connect = true
	c.el.AsyncExecute(c.start)
}

func (c *Connector) Stop() {
	c.connect = false
	if c.tt != nil {
		c.tt.Cancel()
	}
}

func (c *Connector) Restart() {
	c.connect = true
	c.state = connectorDisconnected
	c.retryDelay = 500 * time.Millisecond
	c.start()
}

func (c *Connector) start() {
	if c.connect {
		c.connect0()
	} else {
		logging.Debugf("do not connect")
	}
}

func (c *Connector) connect0() {
	fd, err := sysSocket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		panic(err)
	}

	err = unix.Connect(fd, c.unixSvrAddr)
	logging.Errorf("connect to %s failed due to error: %v", c.svrAddr, err)
	if err != nil {
		switch err {
		case unix.EINPROGRESS, unix.EINTR, unix.EISCONN:
			c.connecting(fd)
		case unix.EAGAIN, unix.EADDRINUSE, unix.EADDRNOTAVAIL, unix.ECONNREFUSED, unix.ENETUNREACH:
			c.retry(fd)
		case unix.EACCES, unix.EPERM, unix.EAFNOSUPPORT, unix.EALREADY, unix.EBADF, unix.EFAULT, unix.ENOTSOCK:
			logging.Errorf("connect to %s failed due to unrecoverable error: %v", c.svrAddr, err)
			_ = unix.Close(fd)
		default:
			logging.Errorf("connect to %s failed due to unrecoverable error: %v", c.svrAddr, err)
			_ = unix.Close(fd)
		}
	} else {
		c.connecting(fd)
	}
}

func (c *Connector) connecting(fd int) {
	c.state = connectorConnecting
	c.ch = NewChannel(c.el, fd)
	c.ch.setWriteCallback(c.handleWrite)
	c.ch.setErrorCallback(c.handleError)
	c.ch.enableWriting()
}

func (c *Connector) handleWrite() {
	logging.Debugf("Connector::handleWrite")
	if c.state == connectorConnecting {
		fd := c.removeAndResetChannel()
		_, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
		if err != nil {
			logging.Warnf("Connector::handleWrite - SO_ERROR : %v", err)
			c.retry(fd)
		} else {
			c.state = connectorConnected
			if c.connect {
				c.cb(fd)
			} else {
				_ = unix.Close(fd)
			}
		}
	} else {
		logging.Errorf("Connector::handleWrite - unexpected state %v", c.state)

	}
}

func (c *Connector) handleError() {
	logging.Errorf("Connector::handleError")
	if c.state == connectorConnecting {
		fd := c.removeAndResetChannel()
		c.retry(fd)
	} else {
		logging.Errorf("Connector::handleError - unexpected state %v", c.state)
	}
}

func (c *Connector) removeAndResetChannel() int {
	c.ch.disableAll()
	c.el.removeChannel(c.ch)
	fd := c.ch.fd
	c.el.AsyncExecute(func() { c.ch = nil })
	return fd
}

func (c *Connector) retry(fd int) {
	_ = unix.Close(fd)
	c.state = connectorDisconnected
	if c.connect {
		logging.Infof("Connector::retry - Retry connecting to %s in %v seconds.", c.svrAddr, c.retryDelay.Seconds())
		c.tt = c.el.AsyncScheduleDelay(c.start, c.retryDelay)
		c.retryDelay = time.Duration(math.Min(float64(c.retryDelay*2), float64(30*time.Second)))
	} else {
		logging.Infof("do not connect")
	}
}

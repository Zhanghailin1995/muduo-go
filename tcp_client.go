package muduo

import (
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"net"
	"strconv"
	"sync"
	"time"
)

type TcpClient struct {
	el              *Eventloop
	connector       *Connector
	onConn          func(*TcpConn)
	onMsg           func(*TcpConn, *Buffer, time.Time)
	onWriteComplete func(*TcpConn)
	retry           bool
	_connect        bool
	nextConnId      uint64
	conn            *TcpConn
	mu              sync.Mutex
}

func NewTcpClient(el *Eventloop, svrAddr string) (*TcpClient, error) {
	connector, err := NewConnector(el, svrAddr, nil)
	if err != nil {
		return nil, err
	}
	cli := &TcpClient{
		el:         el,
		connector:  connector,
		retry:      false,
		_connect:   true,
		nextConnId: 1,
	}
	connector.cb = cli.newConn
	return cli, nil
}

func (c *TcpClient) GetConn() *TcpConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

func (c *TcpClient) SetRetry(retry bool) {
	c.retry = retry
}

func (c *TcpClient) SetOnConn(cb func(*TcpConn)) {
	c.onConn = cb
}

func (c *TcpClient) SetOnMsg(cb func(*TcpConn, *Buffer, time.Time)) {
	c.onMsg = cb
}

func (c *TcpClient) Connect() {
	c._connect = true
	c.connector.Start()
}

func (c *TcpClient) Disconnect() {
	c._connect = false
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.ShutdownWrite()
	}
}

func (c *TcpClient) Stop() {
	c._connect = false
	c.connector.Stop()
}

func (c *TcpClient) Destroy() {
	c._connect = false
	c.connector.Stop()
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		c.el.AsyncExecute(func() {
			conn.setOnClose(func(tcpConn *TcpConn) {
				c.el.AsyncExecute(tcpConn.connectDestroyed)
			})
		})
	} else {
		c.connector.Stop()
		c.el.AsyncScheduleDelay(func() {
			//
		}, 1*time.Second)
	}
}

func (c *TcpClient) newConn(fd int) {
	peerAddr := SockaddrToTCPOrUnixAddr(c.connector.unixSvrAddr)
	localAddr, err := GetLocalAddr(fd)
	if err != nil {
		logging.Errorf("TcpClient::newConn [%s] - failed to get local addr, %v", c.connector.svrAddr, err)
		return
	}
	name := localAddr.String() + "-" + peerAddr.String() + "#" + strconv.FormatUint(c.nextConnId, 10)
	c.nextConnId++
	conn := NewTcpConn(c.el, name, fd, localAddr, peerAddr)
	_ = conn.SetTcpNoDelay(true)
	conn.SetOnConn(c.onConn)
	conn.SetOnMsg(c.onMsg)
	conn.SetOnWriteComplete(c.onWriteComplete)
	conn.setOnClose(c.removeConn)
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	conn.connectEstablished()
}

func (c *TcpClient) removeConn(conn *TcpConn) {
	logging.Debugf("TcpClient::removeConn [%s] - connection %s is down", c.connector.svrAddr, conn.name)
	c.mu.Lock()
	c.conn = nil
	c.mu.Unlock()

	c.el.AsyncExecute(func() {
		conn.connectDestroyed()
	})
	if c.retry && c._connect {
		c.connector.Restart()
	}
}

func GetLocalAddr(fd int) (net.Addr, error) {
	sa, err := unix.Getsockname(fd)
	if err != nil {
		return nil, err
	}
	return SockaddrToTCPOrUnixAddr(sa), nil
}

package muduo

import (
	"muduo/pkg/logging"
	"net"
	"strings"
)

type acceptor struct {
	el        *Eventloop
	so        *socket
	ch        *Channel
	cb        func(int, net.Addr)
	listening bool
}

func newAcceptor(el *Eventloop, addr string, cb func(int, net.Addr)) *acceptor {
	so := newSocket()
	a := &acceptor{
		el:        el,
		so:        so,
		ch:        NewChannel(el, so.fd),
		cb:        cb,
		listening: false,
	}
	_ = so.setReuseAddr(true)
	network, addr := parseProtoAddr(addr)
	err := so.bind(network, addr)
	if err != nil {
		logging.Errorf("bind() failed due to error: %v", err)
	}
	a.ch.setReadCallback(a.handleRead)
	return a
}

func parseProtoAddr(addr string) (network, address string) {
	network = "tcp"
	address = strings.ToLower(addr)
	if strings.Contains(address, "://") {
		pair := strings.Split(address, "://")
		network = pair[0]
		address = pair[1]
	}
	return
}

func (a *acceptor) listen() {
	a.listening = true
	a.so.listen()
	a.ch.enableReading()
}

func (a *acceptor) handleRead() {
	fd, addr, err := a.so.accept()
	if err != nil {
		err = a.so.close()
		if err != nil {
			logging.Errorf("close() failed due to error: %v", err)
		}
		return
	}
	if a.cb != nil {
		a.cb(fd, addr)
	}
}

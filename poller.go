package muduo

import (
	"container/list"
	"golang.org/x/sys/unix"
	"muduo/pkg/logging"
	"muduo/pkg/util"
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"
)

const (
	channelNew = -1
	channelAdd = 1
	channelDel = 2
)

type Poller struct {
	el         *Eventloop
	epollFd    int
	eventList  []epollevent
	channelMap map[int]*Channel
}

func newPoller(el *Eventloop) (poller *Poller, err error) {
	poller = new(Poller)
	if poller.epollFd, err = unix.EpollCreate1(unix.EPOLL_CLOEXEC); err != nil {
		poller = nil
		err = os.NewSyscallError("epoll_create1", err)
		return
	}
	poller.eventList = make([]epollevent, 16)
	poller.channelMap = make(map[int]*Channel)
	poller.el = el
	return
}

func (p *Poller) poll(timeoutMills int) (now time.Time) {
	numEvents, err := epollWait(p.epollFd, p.eventList, timeoutMills)
	now = time.Now()
	if numEvents == 0 || (numEvents < 0 && err == unix.EINTR) {
		logging.Debugf("nothing happened, timeout %d ms", timeoutMills)
		runtime.Gosched()
		return
	} else if err != nil {
		logging.Errorf("error occurs in epoll: %v", os.NewSyscallError("epoll_wait", err))
		return
	}
	if numEvents > 0 {
		logging.Debugf("%d events happened", numEvents)
		p.fillActiveChannels(numEvents, p.el.activeChannels)
		if numEvents == len(p.eventList) {
			p.eventList = make([]epollevent, len(p.eventList)*2)
		}
	}
	return
}

func (p *Poller) fillActiveChannels(numEvents int, activeChannels *list.List) {
	util.Assert(numEvents <= len(p.eventList), "numEvents should not be greater than len(p.eventList)")
	for i := 0; i < numEvents; i++ {
		epollEvt := &p.eventList[i]
		ch := *(**Channel)(unsafe.Pointer(&epollEvt.data))
		ch.revents = epollEvt.events
		ele := activeChannels.PushBack(ch)
		ch.ele = ele
	}
}

func (p *Poller) updateChannel(channel *Channel) {
	logging.Debugf("fd %d events %d", channel.fd, channel.events)
	idx := channel.index
	logging.Debugf("fd = %d index = %d events = %d", channel.fd, idx, channel.events)
	if idx == channelNew || idx == channelDel {
		fd := channel.fd
		if idx == channelNew {
			util.Assert(p.channelMap[fd] == nil, "channelMap should not have fd %d", fd)
			p.channelMap[fd] = channel
		} else {
			util.Assert(p.channelMap[fd] == channel, "channelMap should have fd %d", fd)
		}
		channel.index = channelAdd
		p.update(unix.EPOLL_CTL_ADD, channel)
	} else {
		fd := channel.fd
		util.Assert(p.channelMap[fd] == channel, "channelMap should have fd %d", fd)
		util.Assert(idx == channelAdd, "channel index should be channelAdd")
		if channel.IsNoneEvent() {
			p.update(unix.EPOLL_CTL_DEL, channel)
			channel.index = channelDel
		} else {
			p.update(unix.EPOLL_CTL_MOD, channel)
		}
	}
}

func (p *Poller) update(op int, channel *Channel) {
	var ev epollevent
	ev.events = channel.events
	fd := channel.fd
	*(**Channel)(unsafe.Pointer(&ev.data)) = channel
	logging.Debugf("epoll_ctl op = %d, %s", op, events2String(fd, channel.events))
	err := epollCtl(p.epollFd, op, fd, &ev)
	if err != nil {
		logging.Errorf("epoll_ctl op = %d, fd = %d, %s, err = %v", op, fd, events2String(fd, channel.events), err)
	}
}

func events2String(fd int, events uint32) string {
	res := "fd = " + strconv.Itoa(fd) + " EVENT: "
	if events&unix.EPOLLIN != 0 {
		res += "IN "
	}
	if events&unix.EPOLLPRI != 0 {
		res += "PRI "
	}
	if events&unix.EPOLLOUT != 0 {
		res += "OUT "
	}
	if events&unix.EPOLLHUP != 0 {
		res += "HUP "
	}
	if events&unix.EPOLLRDHUP != 0 {
		res += "RDHUP "
	}
	if events&unix.EPOLLERR != 0 {
		res += "ERR "
	}
	if events&unix.EPOLLET != 0 {
		res += "ET "
	}
	return res

}

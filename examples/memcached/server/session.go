package memcached_server

import (
	"bytes"
	"muduo"
	"muduo/pkg/logging"
	"muduo/pkg/util"
	"strconv"
	"time"
)

type SessionState int

const (
	NewCommand SessionState = iota
	RecvValue
	DiscardValue
)

type ProtocolType int

const (
	Ascii ProtocolType = iota
	Binary
	Auto
)

type Session struct {
	m              *Memcached
	conn           *muduo.TcpConn
	state          SessionState
	protocol       ProtocolType
	command        string
	noreply        bool
	policy         UpdatePolicy
	currItem       *Item
	bytesToDiscard uint64
	needle         *Item
	outputBuffer   *muduo.Buffer
	bytesRead      uint64
	reqProcessed   uint64
}

var LongestKeySize = 250
var LongestKey = string(bytes.Repeat([]byte("x"), LongestKeySize))

func NewSession(m *Memcached, conn *muduo.TcpConn) *Session {
	return &Session{
		m:              m,
		conn:           conn,
		state:          NewCommand,
		protocol:       Ascii,
		noreply:        false,
		policy:         Invalid,
		bytesToDiscard: 0,
		needle:         NewItem(LongestKey, 0, 0, 2, 0),
		bytesRead:      0,
		reqProcessed:   0,
		outputBuffer:   muduo.NewBuffer(),
	}
}

func (s *Session) OnMsg(conn *muduo.TcpConn, buf *muduo.Buffer, t time.Time) {
	initialReadable := buf.ReadableBytes()
	for buf.ReadableBytes() > 0 {
		if s.state == NewCommand {
			if s.protocol == Auto {
				util.Assert(s.bytesRead == 0, "bytesRead should be zero")
				if IsBinaryProtocol(buf.Peek()[0]) {
					s.protocol = Binary
				} else {
					s.protocol = Ascii
				}
			}
			util.Assert(s.protocol == Ascii || s.protocol == Binary, "protocol should be ascii or binary")
			if s.protocol == Ascii {
				search := buf.Search(util.CRLF)
				if search > 0 {
					data := buf.Peek()[:search]
					// TODO process ascii command
					if s.processRequest(data) {
						s.resetRequest()
					}
					buf.Advance(search + len(util.CRLF))
				} else {
					if buf.ReadableBytes() > 1024 {
						conn.ShutdownWrite()
					}
					break
				}
			}
		} else if s.state == RecvValue {
			s.recvValue(buf)
		} else if s.state == DiscardValue {
			s.discardValue(buf)
		} else {
			util.Assert(false, "invalid state")
		}
	}
	s.bytesRead += uint64(initialReadable - buf.ReadableBytes())
}

func (s *Session) recvValue(buf *muduo.Buffer) {
	util.Assert(s.state == RecvValue, "state should be recv value")
	util.Assert(s.currItem != nil, "currItem should not be nil")
	avail := buf.ReadableBytes()
	if s.currItem.NeedBytes() <= avail {
		avail = s.currItem.NeedBytes()
	}
	s.currItem.Append(buf.Peek()[:avail])
	buf.Advance(avail)
	if s.currItem.NeedBytes() == 0 {
		if s.currItem.EndWithCRLF() {
			succ, exists := s.m.Store(s.currItem, s.policy)
			if succ {
				s.Replay([]byte("STORED\r\n"))
			} else {
				if s.policy == Cas {
					if exists {
						s.Replay([]byte("EXISTS\r\n"))
					} else {
						s.Replay([]byte("NOT_FOUND\r\n"))
					}
				} else {
					s.Replay([]byte("NOT_STORED\r\n"))
				}
			}
		} else {
			s.Replay([]byte("CLIENT_ERROR bad data chunk\r\n"))
		}
		s.resetRequest()
		s.state = NewCommand
	}
}

func (s *Session) discardValue(buf *muduo.Buffer) {
	util.Assert(s.state == DiscardValue, "state should be discard value")
	util.Assert(s.currItem == nil, "currItem should be nil")
	if buf.ReadableBytes() < int(s.bytesToDiscard) {
		s.bytesToDiscard -= uint64(buf.ReadableBytes())
		buf.Reset(0)
	} else {
		buf.Advance(int(s.bytesToDiscard))
		s.bytesToDiscard = 0
		s.resetRequest()
		s.state = NewCommand
	}
}

func (s *Session) processRequest(buf []byte) bool {
	util.Assert(s.command == "", "command should be empty")
	util.Assert(s.noreply == false, "noreply should be false")
	util.Assert(s.policy == Invalid, "policy should be invalid")
	util.Assert(s.currItem == nil, "currItem should be nil")
	util.Assert(s.bytesToDiscard == 0, "bytesToDiscard should be zero")
	s.reqProcessed++
	bufLen := len(buf)
	if bufLen >= 8 {
		if string(buf[bufLen-8:]) == " noreply" {
			s.noreply = true
			buf = buf[:bufLen-8]
		}
	}
	tokens := bytes.Fields(buf)
	if len(tokens) == 0 {
		s.Replay([]byte("ERROR\r\n"))
		return true
	}
	idx := 0
	s.command = string(tokens[idx])
	idx++
	switch s.command {
	case "set", "add", "replace", "append", "prepend", "cas":
		return s.DoUpdate(tokens[idx:])
	case "get", "gets":
		cas := s.command == "gets"
		for idx < len(tokens) {
			key := tokens[idx]
			good := len(key) <= 250
			if !good {
				s.Replay([]byte("CLIENT_ERROR bad command line format\r\n"))
				return true
			}
			s.needle.ResetKey(key)
			item, ok := s.m.Get(string(key))
			idx++
			if ok {
				item.Output(s.outputBuffer, cas)
			}
		}
		_, _ = s.outputBuffer.Write([]byte("END\r\n"))
		if s.conn.GetOutboundBuffer().WritableBytes() > 65536+s.outputBuffer.ReadableBytes() {
			logging.Debugf("shrink output buffer from %d to %d", s.outputBuffer.Capacity(), 65536+s.outputBuffer.ReadableBytes())
			s.outputBuffer.Shrink(65536 + s.outputBuffer.ReadableBytes())
		}
		_, _ = s.conn.Write(s.outputBuffer.Next(-1))
	case "delete":
		s.DoDelete(tokens[idx:])
	case "version":
		s.Replay([]byte("VERSION 0.0.1 muduo\r\n"))
	case "quit":
		s.conn.ShutdownWrite()
	case "shutdown":
		s.conn.ShutdownWrite()
		// TODO s.m.Stop()
	default:
		s.Replay([]byte("ERROR\r\n"))
		logging.Errorf("Unknown command: %s", s.command)
	}
	return true
}

func (s *Session) resetRequest() {
	s.command = ""
	s.noreply = false
	s.policy = Invalid
	s.currItem = nil
	s.bytesToDiscard = 0
}

func (s *Session) DoUpdate(tokens [][]byte) bool {
	switch s.command {
	case "set":
		s.policy = Set
	case "add":
		s.policy = Add
	case "replace":
		s.policy = Replace
	case "append":
		s.policy = Append
	case "prepend":
		s.policy = Prepend
	case "cas":
		s.policy = Cas
	default:
		util.Assert(false, "invalid policy")
	}

	idx := 0
	key := tokens[idx]
	idx++
	good := len(key) <= 250

	flags, e1 := strconv.ParseUint(string(tokens[idx]), 10, 32)
	idx++
	exptime, e2 := strconv.ParseInt(string(tokens[idx]), 10, 64)
	idx++
	b, e3 := strconv.ParseInt(string(tokens[idx]), 10, 32)
	idx++

	good = good && e1 == nil && e2 == nil && e3 == nil

	if exptime > 60*60*24*30 {
		exptime = exptime - s.m.startTime().UnixMilli()
		if exptime < 1 {
			exptime = 1
		}
	}
	var cas uint64
	var e4 error
	if good && s.policy == Cas {
		cas, e4 = strconv.ParseUint(string(tokens[idx]), 10, 64)
		idx++
		if e4 != nil {
			good = false
		}
	}
	if !good {
		s.Replay([]byte("CLIENT_ERROR bad command line format\r\n"))
		return true
	}
	if b > 1024*1024 {
		s.Replay([]byte("SERVER_ERROR object too large for cache\r\n"))
		s.needle.ResetKey(key)
		s.m.Delete(string(key))
		s.bytesToDiscard = uint64(b + 2)
		s.state = DiscardValue
		return true
	} else {
		s.currItem = NewItem(string(key), uint32(flags), int(exptime), int(b)+2, cas)
		s.state = RecvValue
		return false
	}
}

func (s *Session) DoDelete(tokens [][]byte) {
	idx := 0
	key := tokens[idx]
	idx++
	good := len(key) <= 250
	if !good {
		s.Replay([]byte("CLIENT_ERROR bad command line format\r\n"))
		return
	} else if idx >= len(tokens) && string(tokens[idx]) != "0" {
		s.Replay([]byte("CLIENT_ERROR bad command line format.  Usage: delete <key> [noreply]\\r\\n"))
		return
	} else {
		s.needle.ResetKey(key)
		if s.m.Delete(string(key)) {
			s.Replay([]byte("DELETED\r\n"))
		} else {
			s.Replay([]byte("NOT_FOUND\r\n"))
		}
	}

}

func (s *Session) Replay(msg []byte) {
	if !s.noreply {
		_ = s.conn.AsyncWrite(msg, nil)
	}
}

func IsBinaryProtocol(first byte) bool {
	return first == 0x80
}

package memcached_server

import (
	"hash/crc64"
	"muduo"
	"strconv"
)

type UpdatePolicy int

var table = crc64.MakeTable(crc64.ISO)

const (
	Invalid UpdatePolicy = iota
	Set
	Add
	Replace
	Append
	Prepend
	Cas
)

type Item struct {
	keylen    int
	flags     uint32
	exptime   int
	valuelen  int
	recvBytes int
	cas       uint64
	_hash     uint64
	data      []byte
}

func NewItem(key string, flags uint32, exptime int, valuelen int, cas uint64) *Item {
	keylen := len([]byte(key))
	it := &Item{
		keylen:    keylen,
		flags:     flags,
		exptime:   exptime,
		valuelen:  valuelen,
		recvBytes: 0,
		cas:       cas,
		_hash:     hash(key),
		data:      make([]byte, keylen+valuelen),
	}
	return it
}

func (it *Item) NeedBytes() int {
	return it.keylen + it.valuelen - it.recvBytes
}

func hash(key string) uint64 {
	return crc64.Checksum([]byte(key), table)
}

func (it *Item) Append(data []byte) {
	copy(it.data[it.recvBytes:], data)
	it.recvBytes += len(data)
}

func (it *Item) EndWithCRLF() bool {
	return it.data[it.recvBytes-2] == '\r' && it.data[it.recvBytes-1] == '\n'
}

func (it *Item) Key() []byte {
	return it.data[:it.keylen]
}

func (it *Item) Value() []byte {
	return it.data[it.keylen:]
}

func (it *Item) ResetKey(key []byte) {
	it.keylen = len(key)
	it.recvBytes = 0
	it.Append(key)
	it._hash = hash(string(key))
}

func (it *Item) Output(outputBuf *muduo.Buffer, needCas bool) {
	_, _ = outputBuf.Write([]byte("VALUE "))
	_, _ = outputBuf.Write(it.data[:it.keylen])
	_, _ = outputBuf.Write([]byte(" "))
	_, _ = outputBuf.Write([]byte(strconv.Itoa(int(it.flags))))
	_, _ = outputBuf.Write([]byte(" "))
	_, _ = outputBuf.Write([]byte(strconv.Itoa(it.valuelen - 2)))
	if needCas {
		_, _ = outputBuf.Write([]byte(" "))
		_, _ = outputBuf.Write([]byte(strconv.Itoa(int(it.cas))))
	}
	_, _ = outputBuf.Write([]byte("\r\n"))
	_, _ = outputBuf.Write(it.Value())
}

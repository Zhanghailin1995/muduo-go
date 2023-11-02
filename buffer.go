package muduo

import "golang.org/x/sys/unix"

type Buffer struct {
	buf        []byte
	readIndex  int
	writeIndex int
}

func NewBuffer() *Buffer {
	return &Buffer{
		buf:        make([]byte, 1024),
		readIndex:  0,
		writeIndex: 0,
	}
}

func (b *Buffer) ReadFd(fd int) (int, error) {
	var extraBuf [65536]byte
	var iov [2][]byte
	iov[0] = b.buf[b.writeIndex:]
	iov[1] = extraBuf[:]
	n, err := unix.Readv(fd, iov[:])
	if err != nil {
		return n, err
	} else if n <= len(b.buf)-b.writeIndex {
		b.writeIndex += n
	} else {
		b.writeIndex = len(b.buf)
		_, _ = b.Write(extraBuf[:n-(len(b.buf)-b.writeIndex)])
	}
	return n, nil
}

func (b *Buffer) Next(n int) []byte {
	if n < 0 || n > b.ReadableBytes() {
		ret := b.buf[b.readIndex:b.writeIndex]
		b.Reset(0)
		return ret
	}
	ret := b.buf[b.readIndex : b.readIndex+n]
	b.readIndex += n
	return ret
}

func (b *Buffer) Read(buf []byte) (int, error) {
	n := copy(buf, b.buf[b.readIndex:b.writeIndex])
	b.readIndex += n
	return n, nil
}

func (b *Buffer) Write(buf []byte) (int, error) {
	b.ensureWritableBytes(len(buf))
	n := copy(b.buf[b.writeIndex:], buf)
	b.writeIndex += n
	return n, nil
}

func (b *Buffer) ReadableBytes() int {
	return b.writeIndex - b.readIndex
}

func (b *Buffer) WritableBytes() int {
	return len(b.buf) - b.writeIndex
}

func (b *Buffer) Peek() []byte {
	return b.buf[b.readIndex:b.writeIndex]
}

func (b *Buffer) Advance(n int) {
	b.readIndex += n
}

func (b *Buffer) Reset(n int) {
	b.readIndex = 0
	b.writeIndex = n
}

func (b *Buffer) ensureWritableBytes(n int) {
	if b.WritableBytes() < n {
		b.buf = append(b.buf, make([]byte, n)...)
	}
}

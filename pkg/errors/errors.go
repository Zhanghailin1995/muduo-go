package errors

import "errors"

var (
	ErrUnsupportedProtocol    = errors.New("unsupported protocol")
	ErrUnsupportedTCPProtocol = errors.New("unsupported TCP protocol")
	ErrAcceptSocket           = errors.New("accept a new connection error")
	ErrConnNotOpened          = errors.New("connection is not opened")
)

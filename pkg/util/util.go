package util

import "fmt"

var CRLF = []byte("\r\n")

func Assert(cond bool, format string, args ...interface{}) {
	if !cond {
		panic(fmt.Sprintf(format, args...))
	}
}

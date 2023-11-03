package muduo

import (
	"muduo/pkg/logging"
	"testing"
	"time"
)

func TestNewEventloopEngine(t *testing.T) {
	eb := NewEventloopEngine("worker")
	el := eb.StartLoop()
	el.AsyncExecute(func() {
		logging.Infof(" ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ hello world ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ ")
	})
	time.Sleep(time.Second * 1)
	el.AsyncScheduleDelay(func() {
		logging.Infof(" ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ hello world after 2 second ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ ")
	}, time.Second*2)
	time.Sleep(time.Second * 3)
	el.AsyncStop()
	time.Sleep(time.Second * 1)

	logging.Infof(" ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ eventloop stopped ^_^ ^_^ ^_^ ^_^ ^_^ ^_^ ")
}

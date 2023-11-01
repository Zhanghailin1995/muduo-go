package muduo

import "sync"

type Functor func()

type EventloopEngine struct {
	el   *Eventloop
	f    Functor
	mu   sync.Mutex
	cond *sync.Cond
}

func NewEventloopEngine() *EventloopEngine {
	eb := &EventloopEngine{}
	eb.cond = sync.NewCond(&eb.mu)
	return eb
}

func (eb *EventloopEngine) StartLoop() *Eventloop {
	go eb.run()

	eb.mu.Lock()
	for eb.el == nil {
		eb.cond.Wait()
	}
	eb.mu.Unlock()
	return eb.el
}

func (eb *EventloopEngine) run() {
	el := NewEventloop()

	eb.mu.Lock()
	eb.el = el
	eb.cond.Broadcast()
	eb.mu.Unlock()

	el.loop()
}

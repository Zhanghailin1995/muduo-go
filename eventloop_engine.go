package muduo

import "sync"

type Functor func()

type EventloopEngine struct {
	id   string
	el   *Eventloop
	f    Functor
	mu   sync.Mutex
	cond *sync.Cond
}

func NewEventloopEngine(id string) *EventloopEngine {
	eb := &EventloopEngine{
		id: id,
	}
	eb.cond = sync.NewCond(&eb.mu)
	return eb
}

func (eng *EventloopEngine) StartLoop() *Eventloop {
	go eng.run()

	eng.mu.Lock()
	for eng.el == nil {
		eng.cond.Wait()
	}
	eng.mu.Unlock()
	return eng.el
}

func (eng *EventloopEngine) run() {
	el := NewEventloop(eng.id)

	eng.mu.Lock()
	eng.el = el
	eng.cond.Broadcast()
	eng.mu.Unlock()

	el.Loop()
}

package muduo

import "strconv"

type EventloopEngineGroup struct {
	boss      *Eventloop
	works     []*Eventloop
	engines   []*EventloopEngine
	engineCnt int
	started   bool
	next      int
}

func NewEventloopEngineGroup(engineCnt int, boss *Eventloop) *EventloopEngineGroup {
	group := &EventloopEngineGroup{
		boss:      boss,
		works:     make([]*Eventloop, 0),
		engines:   make([]*EventloopEngine, 0),
		engineCnt: engineCnt,
		started:   false,
		next:      0,
	}
	return group
}

func (group *EventloopEngineGroup) Start() {
	if !group.started {
		group.started = true
	}
	for i := 0; i < group.engineCnt; i++ {
		engine := NewEventloopEngine("worker-engine-" + strconv.Itoa(i))
		group.engines = append(group.engines, engine)
		group.works = append(group.works, engine.StartLoop())
	}
}

func (group *EventloopEngineGroup) GetNextLoop() *Eventloop {
	if len(group.works) == 0 {
		return group.boss
	}
	if group.next >= len(group.works) {
		group.next = 0
	}
	next := group.works[group.next]
	group.next++
	return next
}

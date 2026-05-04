package events

import (
	"sync"
	"sync/atomic"
)

// Event is anything that can be published on the bus
type Event interface {
	EventName() string
}

// Handlers run synchronously on a dispatch goroutine
type Handler func(Event)

// Bus is an in-process pub/sub event bus.
type Bus struct {
	mu      sync.RWMutex
	subs    map[string][]Handler
	all     []Handler
	queue   chan Event
	stop    chan struct{}
	wg      sync.WaitGroup
	dropped atomic.Uint64
}

// NewBus returns a Bus with the given queue size. Start must be called
// before Publish for events to dispatch.
func NewBus(queueSize int) *Bus {
	if queueSize <= 0 {
		queueSize = 256
	}
	return &Bus{
		subs:  make(map[string][]Handler),
		queue: make(chan Event, queueSize),
		stop:  make(chan struct{}),
	}
}

func (b *Bus) Start() {
	b.wg.Add(1)
	go b.run()
}

func (b *Bus) Stop() {
	close(b.stop)
	b.wg.Wait()
}

// Subscribe registers h to receive events with the given name
func (b *Bus) Subscribe(name string, h Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[name] = append(b.subs[name], h)
	idx := len(b.subs[name]) - 1
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if idx < len(b.subs[name]) {
			b.subs[name][idx] = nil
		}
	}
}

// SubscribeAll registers h to receive every event regardless of name
func (b *Bus) SubscribeAll(h Handler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.all = append(b.all, h)
	idx := len(b.all) - 1
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if idx < len(b.all) {
			b.all[idx] = nil
		}
	}
}

func (b *Bus) Publish(e Event) {
	select {
	case b.queue <- e:
	default:
		b.dropped.Add(1)
	}
}

func (b *Bus) Dropped() uint64 { return b.dropped.Load() }

func (b *Bus) run() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stop:
			return
		case e := <-b.queue:
			b.dispatch(e)
		}
	}
}

func (b *Bus) dispatch(e Event) {
	b.mu.RLock()
	named := append([]Handler(nil), b.subs[e.EventName()]...)
	all := append([]Handler(nil), b.all...)
	b.mu.RUnlock()
	for _, h := range named {
		if h != nil {
			h(e)
		}
	}
	for _, h := range all {
		if h != nil {
			h(e)
		}
	}
}

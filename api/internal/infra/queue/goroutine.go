package queue

import (
	"context"
	"fmt"
	"sync"
)

type GoroutineDispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewGoroutine() *GoroutineDispatcher {
	return &GoroutineDispatcher{handlers: map[string]Handler{}}
}

func (d *GoroutineDispatcher) Register(kind string, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[kind] = h
}

func (d *GoroutineDispatcher) Enqueue(_ context.Context, j Job) error {
	d.mu.RLock()
	h, ok := d.handlers[j.Kind]
	d.mu.RUnlock()
	if !ok {
		return fmt.Errorf("queue: no handler registered for kind %q", j.Kind)
	}
	go func() {
		// New context: the enqueuing request can return before the job
		// runs. Same shape as a River-dispatched job.
		_ = h(context.Background(), j)
	}()
	return nil
}

package app

import (
	"context"
	"sync"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type CancellationRegistry struct {
	mu      sync.Mutex
	cancels map[core.OperationID]context.CancelFunc
}

func NewCancellationRegistry() *CancellationRegistry {
	return &CancellationRegistry{cancels: make(map[core.OperationID]context.CancelFunc)}
}

func (r *CancellationRegistry) Register(id core.OperationID, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, replaced := r.cancels[id]
	r.cancels[id] = cancel
	return replaced
}

func (r *CancellationRegistry) Forget(id core.OperationID) {
	r.mu.Lock()
	delete(r.cancels, id)
	r.mu.Unlock()
}

func (r *CancellationRegistry) Cancel(id core.OperationID) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	delete(r.cancels, id)
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *CancellationRegistry) CancelAll() int {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.cancels))
	for id, cancel := range r.cancels {
		cancels = append(cancels, cancel)
		delete(r.cancels, id)
	}
	r.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

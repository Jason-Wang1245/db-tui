package app

import (
	"sync"
	"time"
)

type Diagnostic struct {
	At        time.Time
	Operation string
	Status    string
}

type Diagnostics struct {
	mu       sync.RWMutex
	capacity int
	entries  []Diagnostic
}

func NewDiagnostics(capacity int) *Diagnostics {
	if capacity < 1 {
		capacity = 1
	}
	return &Diagnostics{capacity: capacity, entries: make([]Diagnostic, 0, capacity)}
}

func (d *Diagnostics) Record(entry Diagnostic) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.entries) == d.capacity {
		copy(d.entries, d.entries[1:])
		d.entries[len(d.entries)-1] = entry
		return
	}
	d.entries = append(d.entries, entry)
}

func (d *Diagnostics) Snapshot() []Diagnostic {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]Diagnostic(nil), d.entries...)
}

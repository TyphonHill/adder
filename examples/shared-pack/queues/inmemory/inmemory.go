// Package inmemory is a queue kept in memory.
package inmemory

// Queue is the in-memory queue.
type Queue struct{ items []string }

// New returns an empty queue.
func New() *Queue { return &Queue{} }

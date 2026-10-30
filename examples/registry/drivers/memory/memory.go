// Package memory keeps values in memory, for tests.
package memory

import (
	"fmt"
	"sync"

	"example.com/registry/storage"
)

type driver struct {
	mu     sync.Mutex
	values map[string][]byte
}

// New returns a driver that keeps values in memory.
func New() storage.Driver {
	return &driver{values: make(map[string][]byte)}
}

func (d *driver) Get(key string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	value, ok := d.values[key]
	if !ok {
		return nil, fmt.Errorf("no value for %q", key)
	}
	return value, nil
}

func (d *driver) Put(key string, value []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.values[key] = value
	return nil
}

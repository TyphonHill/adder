// Package memory is a cache kept in memory.
package memory

// Cache is the memory cache.
type Cache struct{}

// New returns an empty cache.
func New() *Cache { return &Cache{} }

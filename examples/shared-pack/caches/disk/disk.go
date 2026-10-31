// Package disk is a cache kept in files.
package disk

// Cache is the disk cache.
type Cache struct{}

// New returns an empty cache.
func New() *Cache { return &Cache{} }

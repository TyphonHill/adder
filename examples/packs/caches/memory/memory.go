// Package memory is an in-memory cache.
package memory

// Plugin is the memory cache.
type Plugin struct {
	values map[string]string
}

// New returns an empty memory cache.
func New() *Plugin {
	return &Plugin{values: make(map[string]string)}
}

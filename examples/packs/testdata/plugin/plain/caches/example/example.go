// Package example is the Example plugin.
package example

// Plugin is the Example plugin.
type Plugin struct{}

// New returns the Example plugin.
func New() *Plugin {
	return &Plugin{}
}

// Package health defines what a health check does.
package health

// Checker reports whether one dependency of the application is healthy.
type Checker interface {
	Name() string
	Check() error
}

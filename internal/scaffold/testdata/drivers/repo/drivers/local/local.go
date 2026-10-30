package local

import "example.com/storage/driver"

// New returns a driver backed by the local filesystem.
func New(config map[string]string) (driver.Driver, error) { return nil, nil }

// NewOptions is not a constructor and is not registered.
func NewOptions() {}

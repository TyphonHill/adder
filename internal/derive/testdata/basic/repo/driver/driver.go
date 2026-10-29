package driver

// Driver stores and retrieves objects.
type Driver interface {
	Get(key string) ([]byte, error)
}

// Factory creates a Driver from its configuration.
type Factory func(config map[string]string) (Driver, error)

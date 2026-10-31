// Package storage defines what every storage driver does.
package storage

// Driver stores values by key.
type Driver interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
}

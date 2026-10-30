// Package example stores values in an external service.
package example

import (
	"errors"

	"example.com/registry/storage"
)

type driver struct{}

// New returns a example driver.
func New() storage.Driver {
	return driver{}
}

func (driver) Get(key string) ([]byte, error) {
	return nil, errors.New("example: Get is not implemented")
}

func (driver) Put(key string, value []byte) error {
	return errors.New("example: Put is not implemented")
}

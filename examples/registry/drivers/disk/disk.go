// Package disk stores values as files in a directory.
package disk

import (
	"os"
	"path/filepath"

	"example.com/registry/storage"
)

type driver struct{ dir string }

// New returns a driver that stores each value in a file under the system's temporary directory.
func New() storage.Driver {
	return driver{dir: filepath.Join(os.TempDir(), "registry-example")}
}

func (d driver) Get(key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.dir, key))
}

func (d driver) Put(key string, value []byte) error {
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d.dir, key), value, 0o644)
}

// Package checks holds the application's health checks.
package checks

import (
	"errors"
	"os"
)

// Disk checks that the temporary directory is writable. Its methods have value receivers.
type Disk struct{}

func (Disk) Name() string { return "disk" }

func (Disk) Check() error {
	f, err := os.CreateTemp("", "health")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// Database checks the database connection. Its methods have pointer receivers, so only *Database is a
// health.Checker.
type Database struct{ pings int }

func (d *Database) Name() string { return "database" }

func (d *Database) Check() error {
	d.pings++
	return nil
}

// Upstream would be a check, but it has no Name method, so it does not implement health.Checker and the scan
// leaves it out.
type Upstream struct{}

func (Upstream) Check() error { return errors.New("upstream: unreachable") }

package s3

import "example.com/storage/driver"

// New returns a driver backed by an S3 bucket.
func New(config map[string]string) (driver.Driver, error) { return nil, nil }

func newClient() {}

// Package config holds the sections of the application's configuration file.
package config

import "time"

// Server configures the HTTP server.
type Server struct {
	Addr         string        `yaml:"addr"`
	ReadTimeout  time.Duration `yaml:"readTimeout"`
	WriteTimeout time.Duration `yaml:"writeTimeout"`
}

// Database configures the connection to the database.
type Database struct {
	URL      string `yaml:"url"`
	MaxConns int    `yaml:"maxConns"`
}

// Logging configures what the application logs.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// duration is not exported, so the scan leaves it out.
type duration struct{ time.Duration }

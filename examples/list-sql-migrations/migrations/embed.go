// Package migrations holds the database migrations, applied in name order.
package migrations

import "embed"

// Files holds the SQL of every migration, by name.
//
//go:embed *.sql
var Files embed.FS

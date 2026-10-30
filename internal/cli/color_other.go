//go:build !windows

package cli

import "io"

// enableColor reports whether w can show color escape codes, which every terminal outside Windows can. It is a
// variable so tests can pretend.
var enableColor = func(io.Writer) bool { return true }

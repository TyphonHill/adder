//go:build !windows

package cli

import "io"

// enableColor reports whether w can show color escape codes, which every terminal outside Windows can.
func enableColor(io.Writer) bool { return true }

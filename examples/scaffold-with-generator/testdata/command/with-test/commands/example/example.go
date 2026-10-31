// Package example is the example command, which does nothing yet.
package example

import (
	"fmt"
	"io"
)

// Run runs the example command with its arguments.
func Run(w io.Writer, args []string) error {
	_, err := fmt.Fprintln(w, "example: not implemented")
	return err
}

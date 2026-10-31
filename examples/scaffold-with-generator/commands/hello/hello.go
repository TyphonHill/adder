// Package hello is the hello command, which greets its arguments.
package hello

import (
	"fmt"
	"io"
	"strings"
)

// Run runs the hello command with its arguments.
func Run(w io.Writer, args []string) error {
	_, err := fmt.Fprintf(w, "hello, %s\n", strings.Join(args, " and "))
	return err
}

package cli

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// enableColor turns on the console's support for color escape codes, reporting whether it is on.
func enableColor(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	handle := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

// Command adder generates and maintains the repeated pieces of a Go repository.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/TyphonHill/adder/internal/cli"
)

// version is set for release builds with -ldflags "-X main.version=<tag>".
var version string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Main(ctx, buildVersion(), os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// buildVersion falls back to the module version recorded by go install and go tool.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "(devel)"
}

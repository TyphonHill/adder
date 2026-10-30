// Package adder builds adder with packs: derive outputs, templates, and template functions shared across
// repositories.
//
// A pack is usually a Go package that embeds a pack.yaml and its templates:
//
//	//go:embed pack.yaml templates
//	var files embed.FS
//
//	var Pack = adder.Pack{Name: "store", FS: files}
//
// A repository that uses packs builds its own adder from a small main package, and runs it with go tool:
//
//	// tools/adder/main.go
//	package main
//
//	import "github.com/TyphonHill/adder"
//
//	func main() {
//		adder.Main(adder.Pack{
//			Name:  "local",
//			Funcs: map[string]any{"routeName": routeName},
//		})
//	}
//
// Then go get -tool ./tools/adder adds it as a tool, in place of github.com/TyphonHill/adder/cmd/adder, and
// go tool adder runs it.
//
// This API is experimental until v1.0.
package adder

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/TyphonHill/adder/internal/cli"
	"github.com/TyphonHill/adder/internal/pack"
)

// module is this module's path, which build information records with its version.
const module = "github.com/TyphonHill/adder"

// Pack is a set of extensions to adder that repositories share.
type Pack struct {
	// Name identifies the pack: lowercase words joined by hyphens, such as "store". A use in adder.yaml names
	// the pack it uses.
	Name string
	// FS holds the pack's pack.yaml at its root, and the templates it names. It is usually an embed.FS, and nil for
	// a pack of only functions.
	FS fs.FS
	// Funcs are template functions, available to every template, by name. Each follows the rules of
	// text/template and must be deterministic. A name may not be built in or used by another pack.
	Funcs map[string]any
}

// Main runs the adder command line with the given packs, and exits. A pack that cannot be registered, such as
// one with a function named like a built-in one, makes every command fail with an invalid-pack problem.
func Main(packs ...Pack) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Main(ctx, version(), internal(packs), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// internal converts packs to the form the internal packages use.
func internal(packs []Pack) []pack.Pack {
	out := make([]pack.Pack, len(packs))
	for i, p := range packs {
		out[i] = pack.Pack{Name: p.Name, FS: p.FS, Funcs: p.Funcs}
	}
	return out
}

// version returns this module's version from build information, both in adder's own binary and in a repository's
// build that imports it.
func version() string {
	info, _ := debug.ReadBuildInfo()
	return versionIn(info)
}

func versionIn(info *debug.BuildInfo) string {
	if info == nil {
		return "(devel)"
	}
	if info.Main.Path == module {
		return info.Main.Version
	}
	for _, dep := range info.Deps {
		switch {
		case dep.Path != module:
		case dep.Replace != nil:
			return cmp.Or(dep.Replace.Version, "(devel)")
		default:
			return dep.Version
		}
	}
	return "(devel)"
}

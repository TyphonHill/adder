// Package adder builds adder with packs: derive outputs, generators, templates, template functions, and
// scanners shared across repositories.
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
//	import "typhonhill.dev/adder"
//
//	func main() {
//		adder.Main(adder.Pack{
//			Name:  "local",
//			Funcs: map[string]any{"routeName": routeName},
//		})
//	}
//
// Then go get -tool ./tools/adder adds it as a tool, in place of typhonhill.dev/adder/cmd/adder, and
// go tool adder runs it.
//
// This API is experimental until v1.0.
package adder

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"

	"typhonhill.dev/adder/internal/cli"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/pack"
)

// module is this module's path, which build information records with its version.
const module = "typhonhill.dev/adder"

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
	// Scanners are custom scanners, by the name a scan's scanner block gives: lowercase words joined by hyphens,
	// not used by another pack.
	Scanners map[string]Scanner
	// DeprecatedFuncs marks functions of the pack as on their way out, by name, saying what to use instead, such
	// as "use title". They keep working, and adder warns wherever a template calls one.
	DeprecatedFuncs map[string]string
	// DeprecatedScanners marks scanners of the pack as on their way out, by name, saying what to use instead.
	// They keep working, and adder warns wherever a scan names one.
	DeprecatedScanners map[string]string
}

// Scanner returns the items of a scan that names it, for information no built-in scan can find, such as routes
// registered in a call chain or names in a file format adder does not read.
//
// Items are structs, pointers to structs, or map[string]any values, whose fields templates read by name. They
// must be in an order that depends only on the files, which adder keeps. A scanner must not write files or read
// anything but in.FS, so that adder check and adder derive agree.
//
// A returned Problem, or several joined with errors.Join, is reported at its file; any other error is reported
// at the scan.
type Scanner func(ctx context.Context, in *ScanInput) ([]any, error)

// ScanInput is what a scanner reads.
type ScanInput struct {
	// FS is the repository, including the files adder new is about to write.
	FS fs.FS
	// Dirs are the directories the scan's dirs match, slash-separated, with the boundaries of every scan, in
	// order. It is empty for a scan without dirs.
	Dirs []string

	decode func(any) error
}

// Decode decodes the scan's with settings into v, a pointer, rejecting fields v does not have. Its problems
// point at the settings in adder.yaml. Without settings, as for a ScanInput a scanner's own tests build, v is
// left as it is.
func (in *ScanInput) Decode(v any) error {
	if in.decode == nil {
		return nil
	}
	return in.decode(v)
}

// Problem is a problem a scanner found at a position in a repository file.
type Problem struct {
	// File is slash-separated and relative to the repository root.
	File string
	// Line and Column are 1-based; zero means unknown.
	Line, Column int
	Message      string
	// Hint says how to fix the problem, if that is known.
	Hint string
}

func (p Problem) Error() string {
	if pos := (diag.Pos{File: p.File, Line: p.Line, Column: p.Column}).String(); pos != "" {
		return pos + ": " + p.Message
	}
	return p.Message
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
		out[i] = pack.Pack{Name: p.Name, FS: p.FS, Funcs: p.Funcs, Scanners: make(map[string]pack.Scanner, len(p.Scanners)), DeprecatedFuncs: p.DeprecatedFuncs, DeprecatedScanners: p.DeprecatedScanners}
		for name, s := range p.Scanners {
			out[i].Scanners[name] = wrap(s)
		}
	}
	return out
}

// wrap adapts a scanner to the internal packages, reporting the Problems it returns as diagnostics.
func wrap(s Scanner) pack.Scanner {
	if s == nil {
		return nil
	}
	return func(ctx context.Context, in pack.ScanInput) ([]any, error) {
		items, err := s(ctx, &ScanInput{FS: in.FS, Dirs: in.Dirs, decode: in.Decode})
		if err == nil {
			return items, nil
		}
		return nil, problems(err)
	}
}

// problems returns the Problems in err, a Problem or several joined, as scanner diagnostics. Other errors are
// kept, to be reported at the scan.
func problems(err error) diag.List {
	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		var list diag.List
		for _, err := range joined.Unwrap() {
			list = append(list, problems(err)...)
		}
		return list
	}
	p, ok := errors.AsType[Problem](err)
	if ptr, isPtr := errors.AsType[*Problem](err); !ok && isPtr && ptr != nil {
		p, ok = *ptr, true
	}
	if !ok {
		return diag.From(err)
	}
	return diag.List{{Code: diag.Scanner, Pos: diag.Pos{File: p.File, Line: p.Line, Column: p.Column}, Message: p.Message, Hint: p.Hint}}
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

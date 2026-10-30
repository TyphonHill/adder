package scan

import (
	"go/ast"
	"go/build/constraint"
	"go/token"
	"regexp"
	"slices"
	"strings"

	"typhonhill.dev/adder/internal/diag"
)

// platformTags are the build tags set by the platform and toolchain, from Go's internal/syslist, which may take
// any value. Every other tag is a custom tag, false unless a scan's buildTags lists it.
var platformTags = func() map[string]bool {
	m := make(map[string]bool)
	for tag := range strings.FieldsSeq(
		// GOOS values.
		"aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos " +
			// GOARCH values.
			"386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le " +
			"ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm " +
			// Tags the toolchain sets.
			"unix cgo gc gccgo") {
		m[tag] = true
	}
	return m
}()

var goVersionTag = regexp.MustCompile(`^go1\.[0-9]+$`)

func isPlatformTag(tag string) bool {
	return platformTags[tag] || goVersionTag.MatchString(tag) || strings.HasPrefix(tag, "goexperiment.")
}

// maxPlatformTags bounds the platform tags tried together; a file using more is assumed to build somewhere.
const maxPlatformTags = 16

// buildable reports whether some platform can build file, a file in fset, given its build constraints and the
// custom tags that are set. Platform tags may take any value, so the answer is the same on every machine.
func buildable(fset *token.FileSet, file *ast.File, tags []string) (bool, error) {
	expr, err := buildConstraint(fset, file)
	if expr == nil || err != nil {
		return true, err
	}
	var platform []string
	expr.Eval(func(tag string) bool {
		if isPlatformTag(tag) && !slices.Contains(platform, tag) {
			platform = append(platform, tag)
		}
		return false
	})
	if len(platform) > maxPlatformTags {
		return true, nil
	}
	for set := range 1 << len(platform) {
		if expr.Eval(func(tag string) bool {
			if i := slices.Index(platform, tag); i >= 0 {
				return set&(1<<i) != 0
			}
			return slices.Contains(tags, tag)
		}) {
			return true, nil
		}
	}
	return false, nil
}

// buildConstraint returns the file's build constraint: its //go:build line, or its // +build lines, which
// combine with "and", when it has no //go:build line. It returns nil when the file has neither.
func buildConstraint(fset *token.FileSet, file *ast.File) (constraint.Expr, error) {
	var goBuild, plusBuild constraint.Expr
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, c := range group.List {
			isGoBuild := constraint.IsGoBuild(c.Text)
			if !isGoBuild && !constraint.IsPlusBuild(c.Text) {
				continue
			}
			if isGoBuild && goBuild != nil {
				return nil, syntaxProblem(fset, c, "a file can have only one //go:build line")
			}
			parsed, err := constraint.Parse(c.Text)
			if err != nil {
				return nil, syntaxProblem(fset, c, "invalid build constraint: "+err.Error())
			}
			switch {
			case isGoBuild:
				goBuild = parsed
			case plusBuild == nil:
				plusBuild = parsed
			default:
				plusBuild = &constraint.AndExpr{X: plusBuild, Y: parsed}
			}
		}
	}
	if goBuild != nil {
		return goBuild, nil
	}
	return plusBuild, nil
}

func syntaxProblem(fset *token.FileSet, c *ast.Comment, message string) *diag.Diagnostic {
	pos := fset.Position(c.Pos())
	return &diag.Diagnostic{
		Code:    diag.GoSyntax,
		Pos:     diag.Pos{File: pos.Filename, Line: pos.Line, Column: pos.Column},
		Message: message,
	}
}

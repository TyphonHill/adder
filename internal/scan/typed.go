package scan

import (
	"context"
	"go/ast"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/header"
)

// TypedSource is what a type-aware scan reads.
type TypedSource struct {
	// FS is the repository, with any new files overlaid.
	FS fs.FS
	// Dir is the repository's directory on disk, which the Go toolchain reads.
	Dir string
	// Overlay holds files that are not on disk yet, by slash-separated path, such as those adder new creates.
	Overlay map[string][]byte
}

// The platform a type-aware scan checks unless it sets one, the same on every machine.
const (
	defaultGOOS   = "linux"
	defaultGOARCH = "amd64"
)

// RunTyped is Run for a scan with typeCheck: it loads the scanned packages with the Go toolchain, offline and in
// one fixed environment, and applies the filters that compare types.
func RunTyped(ctx context.Context, src TypedSource, s config.Scan) ([]Item, error) {
	l := newListing(src.FS)
	dirs, err := expandDirs(l, s)
	if err != nil {
		return nil, err
	}
	if src.Dir == "" {
		return nil, diag.Errorf(diag.TypeCheck, diag.Pos{}, "type-aware scans read the repository from disk, and it has no directory")
	}
	root, err := filepath.Abs(src.Dir)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: src.Dir}, err)
	}

	scanned := make(map[string]bool)
	var patterns []string
	for _, dir := range dirs {
		if hasPackage(l, dir, s.IncludeTests) {
			scanned[dir] = true
			patterns = append(patterns, "./"+dir)
		}
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	ifacePath, ifaceName := "", ""
	if s.Types != nil && s.Types.Implements != "" && s.Types.Implements != "error" {
		i := strings.LastIndex(s.Types.Implements, ".")
		ifacePath, ifaceName = s.Types.Implements[:i], s.Types.Implements[i+1:]
		patterns = append(patterns, ifacePath)
	}

	overlay := make(map[string][]byte, len(src.Overlay))
	for name, content := range src.Overlay {
		overlay[filepath.Join(root, filepath.FromSlash(name))] = content
	}
	var buildFlags []string
	if len(s.BuildTags) > 0 {
		buildFlags = []string{"-tags=" + strings.Join(s.BuildTags, ",")}
	}
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedForTest,
		Dir:        root,
		Env:        goEnv(root, s.Platform),
		BuildFlags: buildFlags,
		Overlay:    overlay,
		Tests:      s.IncludeTests,
	}, patterns...)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, &diag.Diagnostic{
			Code:    diag.TypeCheck,
			Message: "loading packages: " + err.Error(),
			Hint:    "type-aware scans need the go command on PATH and a go.mod at the root",
			Err:     err,
		}
	}

	if err := loadErrors(root, pkgs).Err(); err != nil {
		return nil, err
	}

	filter := &typeFilter{}
	if s.Types != nil && s.Types.Implements != "" {
		if filter.iface, err = lookupInterface(pkgs, ifacePath, ifaceName, s.Types.Implements); err != nil {
			return nil, err
		}
	}

	var items []Item
	for _, pkg := range selectPackages(pkgs, s.IncludeTests) {
		for i, file := range pkg.Syntax {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(root, pkg.CompiledGoFiles[i])
			if err != nil {
				continue
			}
			name := filepath.ToSlash(rel)
			dir := path.Dir(name)
			if !scanned[dir] {
				continue // the interface's own package, or a file outside the scanned directories
			}
			if _, generated := header.Source(file); generated {
				continue
			}
			filter.info, filter.pkg = pkg.TypesInfo, pkg.Types
			base := Item{Package: file.Name.Name, ImportPath: pkg.PkgPath, Dir: dir, File: name}
			found, err := declItems(pkg.Fset, file, s, base, filter)
			if err != nil {
				return nil, diag.Locate(err, "", diag.Pos{})
			}
			items = append(items, found...)
		}
	}
	sortItems(items)
	return items, nil
}

// hasPackage reports whether dir holds Go files the scan would load.
func hasPackage(l *listing, dir string, includeTests bool) bool {
	entries, err := l.read(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		name := e.Name()
		return e.Type().IsRegular() && strings.HasSuffix(name, ".go") && (includeTests || !strings.HasSuffix(name, "_test.go"))
	})
}

// selectPackages returns the packages to take items from. With tests, a package's test variant replaces it,
// since it holds the same files and the test files.
func selectPackages(pkgs []*packages.Package, includeTests bool) []*packages.Package {
	if !includeTests {
		return pkgs
	}
	tested := make(map[string]bool)
	for _, pkg := range pkgs {
		if pkg.ForTest != "" {
			tested[pkg.ForTest] = true
		}
	}
	var out []*packages.Package
	for _, pkg := range pkgs {
		switch {
		case strings.HasSuffix(pkg.ID, ".test"): // the generated test main
		case pkg.ForTest == "" && tested[pkg.PkgPath]:
		default:
			out = append(out, pkg)
		}
	}
	return out
}

// goEnv returns the environment for the go command: the caller's, with every setting that would change which
// files are checked, or download anything, fixed.
func goEnv(root, platform string) []string {
	goos, goarch := defaultGOOS, defaultGOARCH
	if platform != "" {
		goos, goarch, _ = strings.Cut(platform, "/")
	}
	work := "off"
	if exists(filepath.Join(root, "go.work")) {
		work = filepath.Join(root, "go.work")
	}
	mod := "-mod=readonly"
	if exists(filepath.Join(root, "vendor", "modules.txt")) {
		mod = "-mod=vendor"
	}
	fixed := []string{
		"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0", "GOFLAGS=" + mod, "GOPROXY=off",
		"GOTOOLCHAIN=auto", "GOWORK=" + work, "GO111MODULE=on", "GOEXPERIMENT=",
	}
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(fixed, func(f string) bool { return strings.HasPrefix(f, key+"=") }) {
			env = append(env, kv)
		}
	}
	return append(env, fixed...)
}

func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// loadErrors returns the problems the toolchain reported for the packages and everything they import, each once.
// A directory whose files are all for other platforms is not a problem, since a scan of it finds nothing.
func loadErrors(root string, pkgs []*packages.Package) diag.List {
	var problems diag.List
	seen := make(map[string]bool)
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		typeErrors := slices.ContainsFunc(pkg.Errors, func(e packages.Error) bool { return e.Kind == packages.TypeError })
		for _, e := range pkg.Errors {
			switch {
			case strings.Contains(e.Msg, "build constraints exclude all Go files"):
				continue
			case e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# ") && typeErrors:
				continue // the compiler's copy of the type checker's errors
			}
			d := loadError(root, e)
			if key := d.Error(); !seen[key] {
				seen[key] = true
				problems = append(problems, d)
			}
		}
	})
	// A package that cannot be imported also fails to type check where it is imported; report only the cause.
	if slices.ContainsFunc(problems, func(d *diag.Diagnostic) bool { return !strings.HasPrefix(d.Message, "could not import ") }) {
		problems = slices.DeleteFunc(problems, func(d *diag.Diagnostic) bool { return strings.HasPrefix(d.Message, "could not import ") })
	}
	return problems
}

// loadError turns a problem the toolchain reported into a diagnostic with a position relative to the root.
func loadError(root string, e packages.Error) *diag.Diagnostic {
	msg, pos := e.Msg, e.Pos
	if first, rest, ok := strings.Cut(msg, "\n"); ok && strings.HasPrefix(first, "# ") && pos == "" {
		// The compiler's output: "# package" and then "file:line:col: message".
		if at, text, ok := strings.Cut(rest, ": "); ok {
			if _, line, _ := splitPos(at); line > 0 {
				msg, pos = text, at
			}
		}
	}
	d := &diag.Diagnostic{Code: diag.TypeCheck, Message: msg}
	for _, missing := range []string{"cannot find module providing package", "no required module provides", "missing go.sum entry", "GOPROXY=off"} {
		if strings.Contains(msg, missing) {
			d.Hint = "adder never downloads modules; run go mod download, then try again"
		}
	}
	file, line, col := splitPos(pos)
	if file == "" {
		return d
	}
	if filepath.IsAbs(file) {
		if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
			file = rel
		}
	}
	d.Pos = diag.Pos{File: filepath.ToSlash(file), Line: line, Column: col}
	return d
}

// splitPos splits a position the toolchain reports, "file:line:col" or "file:line", where the file name may
// contain colons, as on Windows.
func splitPos(pos string) (file string, line, col int) {
	if pos == "" || pos == "-" {
		return "", 0, 0
	}
	parts := strings.Split(pos, ":")
	nums := 0
	for nums < 2 && nums < len(parts)-1 {
		if _, err := strconv.Atoi(parts[len(parts)-1-nums]); err != nil {
			break
		}
		nums++
	}
	file = strings.Join(parts[:len(parts)-nums], ":")
	if nums >= 1 {
		line, _ = strconv.Atoi(parts[len(parts)-nums])
	}
	if nums == 2 {
		col, _ = strconv.Atoi(parts[len(parts)-1])
	}
	return file, line, col
}

// lookupInterface finds the interface a scan's implements names among the loaded packages.
func lookupInterface(pkgs []*packages.Package, pkgPath, name, written string) (*types.Interface, error) {
	if written == "error" {
		return types.Universe.Lookup("error").Type().Underlying().(*types.Interface), nil
	}
	for _, pkg := range pkgs {
		if pkg.PkgPath != pkgPath || pkg.Types == nil {
			continue
		}
		obj, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			break
		}
		if iface, ok := obj.Type().Underlying().(*types.Interface); ok {
			return iface, nil
		}
		return nil, diag.Errorf(diag.TypeCheck, diag.Pos{}, "implements: %s is not an interface", written)
	}
	return nil, diag.Errorf(diag.TypeCheck, diag.Pos{}, "implements: %s does not exist", written)
}

// typeFilter applies the filters that compare types. A nil filter compares types as written, for scans that only
// parse.
type typeFilter struct {
	info  *types.Info
	pkg   *types.Package
	iface *types.Interface
}

// typeStrings returns t written with full import paths, and with its own package's types unqualified, so a
// filter may name a type in the scanned package either way.
func (f *typeFilter) typeStrings(t types.Type) (full, local string) {
	t = types.Unalias(t)
	full = types.TypeString(t, func(p *types.Package) string { return p.Path() })
	local = types.TypeString(t, func(p *types.Package) string {
		if p == f.pkg {
			return ""
		}
		return p.Path()
	})
	return full, local
}

func (f *typeFilter) is(t types.Type, want string) bool {
	full, local := f.typeStrings(t)
	return sameType(full, want) || sameType(local, want)
}

// firstParamIs reports whether fn's first parameter has the type want, compared by type rather than spelling.
func (f *typeFilter) firstParamIs(fn *ast.FuncDecl, want string) bool {
	obj, ok := f.info.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	params := obj.Type().(*types.Signature).Params()
	return params.Len() > 0 && f.is(params.At(0).Type(), want)
}

// valueTypeIs reports whether the constant or variable name has the type want. Without type information it
// compares written, the type as written.
func (f *typeFilter) valueTypeIs(name *ast.Ident, want, written string) bool {
	if f == nil {
		return want == written
	}
	obj := f.info.Defs[name]
	return obj != nil && f.is(obj.Type(), want)
}

// implements reports whether the type declared by ts implements the scan's interface, and whether only a pointer
// to it does. Interfaces and generic types are never selected.
func (f *typeFilter) implements(ts *ast.TypeSpec) (ok, pointer bool) {
	if f == nil || f.iface == nil {
		return false, false
	}
	obj, isType := f.info.Defs[ts.Name].(*types.TypeName)
	if !isType || types.IsInterface(obj.Type()) || ts.TypeParams != nil {
		return false, false
	}
	t := obj.Type()
	if types.Implements(t, f.iface) {
		return true, false
	}
	return types.Implements(types.NewPointer(t), f.iface), true
}

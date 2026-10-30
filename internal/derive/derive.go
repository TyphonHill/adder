// Package derive rebuilds files whose content follows from the repository's source code.
package derive

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/header"
	"github.com/TyphonHill/adder/internal/pack"
	"github.com/TyphonHill/adder/internal/plan"
	"github.com/TyphonHill/adder/internal/render"
	"github.com/TyphonHill/adder/internal/scan"
)

// Sources are the file systems a plan reads from.
type Sources struct {
	// Repo is the repository root holding the scanned code and go.mod.
	Repo fs.FS
	// Templates is the directory holding the configuration file; template paths resolve against it.
	Templates fs.FS
	// Existing holds the current outputs. Nil means none exist yet.
	Existing fs.FS

	// RepoDir, TemplatesDir, and ExistingDir name the directories behind each file system. They affect the file
	// paths reported in diagnostics, and type-aware scans read the repository from RepoDir, so it must be set
	// for them.
	RepoDir, TemplatesDir, ExistingDir string

	// Overlay holds files Repo has that are not on disk yet, by slash-separated path, for type-aware scans.
	Overlay map[string][]byte

	// Packs are the packs adder was built with. Nil means none.
	Packs *pack.Registry
}

// unit is the scans and outputs of a configuration, or of one use of a pack, with where its templates are.
type unit struct {
	cfg       *config.Config
	templates fs.FS
	dir       string // the directory behind templates, for the configuration's own
	pack      string // the pack, for a use
	use       string
	options   map[string]any
}

// units returns the configuration's own unit, then one for each use of a pack, in name order.
func units(cfg *config.Config, src Sources) []unit {
	all := []unit{{cfg: cfg, templates: src.Templates, dir: src.TemplatesDir}}
	for _, name := range slices.Sorted(maps.Keys(cfg.Packs)) {
		use := cfg.Packs[name]
		all = append(all, unit{cfg: use.Config, templates: use.FS, pack: use.Pack, use: name, options: use.Options})
	}
	return all
}

// packUnit is the unit whose templates are those of a pack the configuration's own outputs or files render. It
// has no scans of its own.
func packUnit(cfg *config.Config, pack string) unit {
	pt := cfg.PackTemplates[pack]
	return unit{cfg: &config.Config{Partials: pt.Partials}, templates: pt.FS, pack: pack}
}

// PackTemplates parses the templates and partials of a pack the configuration's own outputs or generator files
// render.
func PackTemplates(cfg *config.Config, src Sources, pack string) (*render.Set, error) {
	return templates(packUnit(cfg, pack), src)
}

// locate returns the problems in err with positions in the unit's templates usable from the current directory,
// attributing those without a position to decl.
func (u unit) locate(err error, decl diag.Pos) diag.List {
	if u.pack != "" {
		return diag.InPack(err, u.pack, decl)
	}
	return diag.Locate(err, u.dir, decl)
}

// prepared is a unit with its templates parsed and its scans run.
type prepared struct {
	unit
	set  *render.Set
	data map[string]any
}

// Plan computes every derived output and how it differs from the existing files, without writing anything.
// It reports every problem it finds rather than stopping at the first.
func Plan(ctx context.Context, cfg *config.Config, src Sources) (*plan.Plan, error) {
	var all []prepared
	var problems diag.List
	for _, u := range units(cfg, src) {
		set, data, err := prepare(ctx, u, src)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		problems = append(problems, diag.From(err)...)
		all = append(all, prepared{u, set, data})
	}
	borrowed := make(map[string]prepared, len(cfg.PackTemplates))
	for _, pack := range slices.Sorted(maps.Keys(cfg.PackTemplates)) {
		u := packUnit(cfg, pack)
		set, err := templates(u, src)
		problems = append(problems, diag.From(err)...)
		borrowed[pack] = prepared{unit: u, set: set}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}

	// Single outputs claim their paths first, so an item rendering onto one is reported wherever it appears.
	paths := make(map[string]bool)
	for _, u := range all {
		for _, out := range u.cfg.Derive.Outputs {
			if out.Each == "" {
				paths[out.Path] = true
			}
		}
	}
	var p plan.Plan
	for _, u := range all {
		for _, out := range u.cfg.Derive.Outputs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			r := u
			if out.Pack != "" && u.pack == "" {
				r = borrowed[out.Pack]
				r.data = u.data // a pack's template rendered with the configuration's own data
			}
			if out.Each == "" {
				problems = append(problems, renderOutput(r, r.data, src, out, out.Path, &p)...)
				continue
			}
			found, err := planEach(ctx, r, src, out, &p, paths)
			if err != nil {
				return nil, err
			}
			problems = append(problems, found...)
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return &p, nil
}

// renderOutput adds the output rendered to target to p.
func renderOutput(u prepared, data map[string]any, src Sources, out config.Output, target string, p *plan.Plan) diag.List {
	content, err := u.set.File(out.Template, target, data)
	if err == nil {
		content, err = header.Ensure(target, content, out.Source())
	}
	if err != nil {
		return u.locate(err, out.Pos)
	}
	if err := p.Add(src.Existing, target, content); err != nil {
		return diag.Locate(err, src.ExistingDir, out.Pos)
	}
	if change := p.Changes[len(p.Changes)-1]; change.Status == plan.Updated && !replaceable(change) {
		p.Changes = p.Changes[:len(p.Changes)-1]
		return diag.Locate(&diag.Diagnostic{
			Code:    diag.FileExists,
			Pos:     diag.Pos{File: target},
			Message: fmt.Sprintf("file already exists and adder did not generate it, so it is not replaced by %s", out.Source()),
			Hint:    "adder never overwrites files people own; move or delete the file, or write the output somewhere else",
		}, src.ExistingDir, out.Pos)
	}
	return nil
}

// replaceable reports whether derive may overwrite a file with an output: only when the file has adder's header,
// naming any template so that templates can be renamed. Files of a type without comments, such as JSON, get no
// header, so adder cannot tell its own apart and replaces them as before.
func replaceable(c plan.Change) bool {
	if !header.Supported(c.Path) {
		return true
	}
	_, ours := header.Parse(c.Path, c.Old)
	return ours
}

// planEach adds the output rendered for each item of its scan to p, then the files it generated before that it
// no longer produces. Paths holds every path planned so far.
func planEach(ctx context.Context, u prepared, src Sources, out config.Output, p *plan.Plan, paths map[string]bool) (diag.List, error) {
	items, _ := u.data[out.Each].([]scan.Item)
	produced := make(map[string]bool, len(items))
	var problems diag.List
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		itemData := maps.Clone(u.data)
		itemData["Item"] = item
		target, err := itemPath(u.set, out, item, itemData, paths)
		if err != nil {
			problems = append(problems, diag.From(err)...)
			continue
		}
		paths[target], produced[target] = true, true
		problems = append(problems, renderOutput(u, itemData, src, out, target, p)...)
	}
	if len(problems) > 0 {
		return problems, nil
	}

	stale, err := staleFiles(ctx, src.Existing, out, produced)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil {
		return diag.Locate(err, src.ExistingDir, out.Pos), nil
	}
	p.Changes = append(p.Changes, stale...)
	return nil, nil
}

// itemPath renders the output's path for an item and checks that it is a new file name inside the root.
func itemPath(set *render.Set, out config.Output, item scan.Item, data map[string]any, paths map[string]bool) (string, error) {
	field := fmt.Sprintf("output %q: path %q", out.Template, out.Path)
	rendered, err := set.Text(out.Path, data)
	if err != nil {
		return "", diag.Prefix(err, out.Pos, field+": ")
	}
	target := path.Clean(rendered)
	field += fmt.Sprintf(" renders as %q for %s in %s, which", rendered, item.Name, item.File)
	switch {
	case strings.TrimSpace(rendered) == "" || strings.HasSuffix(rendered, "/") || target == ".":
		return "", diag.Errorf(diag.InvalidValue, out.Pos, "%s is not a file name", field)
	case paths[target]:
		return "", &diag.Diagnostic{
			Code:    diag.DuplicatePath,
			Pos:     out.Pos,
			Message: field + " is also written by another item or output",
			Hint:    "use the item in the path, such as {{ .Item.Dir }}/{{ .Item.Name | toSnakeCase }}.gen.go",
		}
	}
	if d := config.CheckPath(out.Pos, field, target); d != nil {
		return "", d
	}
	return target, nil
}

// staleFiles finds the files the output generated that it no longer produces: files matching its path, whose
// header names its template. It skips the directories ** skips, and never follows symbolic links.
func staleFiles(ctx context.Context, existing fs.FS, out config.Output, produced map[string]bool) ([]plan.Change, error) {
	if existing == nil {
		return nil, nil
	}
	pattern, dir, err := render.PathPattern(out.Path)
	if err != nil {
		return nil, err
	}
	if err := plan.CheckLinks(existing, dir); err != nil {
		return nil, err
	}
	var stale []plan.Change
	err = fs.WalkDir(existing, dir, func(name string, entry fs.DirEntry, err error) error {
		switch {
		case name == dir && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case err != nil:
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		case ctx.Err() != nil:
			return ctx.Err()
		case entry.IsDir():
			if name != dir && (scan.Ignored(entry.Name()) || scan.Boundary(existing, name)) {
				return fs.SkipDir
			}
			return nil
		case !entry.Type().IsRegular() || produced[name] || !pattern.MatchString(name):
			return nil
		}
		content, err := fs.ReadFile(existing, name)
		if err != nil {
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		}
		if source, ok := header.Parse(name, content); ok && source == out.Source() {
			stale = append(stale, plan.Change{Path: name, Status: plan.Removed, Old: content})
		}
		return nil
	})
	return stale, err
}

// Prepare parses the configuration's own partials and runs its scans, returning the templates and the data they
// render with. With use set, it does the same for that use of a pack instead.
func Prepare(ctx context.Context, cfg *config.Config, src Sources, use string) (*render.Set, map[string]any, error) {
	all := units(cfg, src)
	i := slices.IndexFunc(all, func(u unit) bool { return u.use == use })
	if i < 0 {
		return nil, nil, fmt.Errorf("no pack use %q", use)
	}
	return prepare(ctx, all[i], src)
}

// prepare parses a unit's partials and runs its scans.
func prepare(ctx context.Context, u unit, src Sources) (*render.Set, map[string]any, error) {
	set, err := templates(u, src)
	problems := diag.From(err)
	data, err := templateData(ctx, u, src)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, nil, err
	}
	problems = append(problems, diag.From(err)...)
	if err := problems.Err(); err != nil {
		return nil, nil, err
	}
	return set, data, nil
}

// templates parses a unit's partials into the set its outputs render with.
func templates(u unit, src Sources) (*render.Set, error) {
	var files []string
	var problems diag.List
	for _, partial := range u.cfg.Partials {
		matches, err := fs.Glob(u.templates, partial.Pattern)
		if err != nil {
			problems = append(problems, diag.Wrap(diag.InvalidValue, partial.Pos, err))
			continue
		}
		matches = slices.DeleteFunc(matches, func(name string) bool {
			info, err := fs.Stat(u.templates, name)
			return err == nil && info.IsDir()
		})
		if len(matches) == 0 {
			problems = append(problems, &diag.Diagnostic{
				Code:    diag.NoMatch,
				Pos:     partial.Pos,
				Message: fmt.Sprintf("partial %q matches no files", partial.Pattern),
				Hint:    partialsHint(u),
			})
			continue
		}
		files = append(files, matches...)
	}

	slices.Sort(files)
	set, err := render.NewSet(u.templates, slices.Compact(files), src.Packs.Funcs())
	if err != nil {
		problems = append(problems, u.locate(err, diag.Pos{})...)
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return set, nil
}

// templateData runs a unit's scans and adds the module path when the repository has a go.mod, and the vars of
// the configuration's own unit or the options of a use.
func templateData(ctx context.Context, u unit, src Sources) (map[string]any, error) {
	cfg := u.cfg
	data := make(map[string]any, len(cfg.Derive.Scans)+2)
	if u.pack != "" {
		data["Options"] = u.options
	} else {
		vars := cfg.Vars
		if vars == nil {
			vars = map[string]string{}
		}
		data["Vars"] = vars
	}

	gomod, err := fs.ReadFile(src.Repo, "go.mod")
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, diag.Locate(diag.Wrap(diag.IO, diag.Pos{File: "go.mod"}, err), src.RepoDir, diag.Pos{})
	}
	module := modfile.ModulePath(gomod)
	if module != "" {
		data["Module"] = module
	}

	var problems diag.List
	for _, name := range slices.Sorted(maps.Keys(cfg.Derive.Scans)) {
		s := cfg.Derive.Scans[name]
		var items []scan.Item
		if s.TypeCheck {
			items, err = scan.RunTyped(ctx, scan.TypedSource{FS: src.Repo, Dir: src.RepoDir, Overlay: src.Overlay}, s)
		} else {
			items, err = scan.Run(ctx, src.Repo, module, s)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if err != nil {
			problems = append(problems, diag.Locate(err, src.RepoDir, s.Pos)...)
			continue
		}
		data[name] = items
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func partialsHint(u unit) string {
	if u.pack != "" {
		return "partial patterns in pack.yaml are relative to the pack's root"
	}
	return "partial patterns are relative to the directory holding adder.yaml"
}

// Open returns sources for the repository at root, whose existing outputs are in the root too, and a function
// that closes them.
func Open(cfg *config.Config, root string, packs *pack.Registry) (Sources, func(), error) {
	repo, err := os.OpenRoot(root)
	if err != nil {
		return Sources{}, nil, diag.Wrap(diag.IO, diag.Pos{File: root}, err)
	}
	templates, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		_ = repo.Close()
		return Sources{}, nil, diag.Wrap(diag.IO, diag.Pos{File: cfg.Dir}, err)
	}
	src := Sources{
		Repo: repo.FS(), RepoDir: root,
		Templates: templates.FS(), TemplatesDir: cfg.Dir,
		Existing: repo.FS(), ExistingDir: root,
		Packs: packs,
	}
	// Both roots are only read, so closing them cannot lose anything.
	return src, func() { _ = repo.Close(); _ = templates.Close() }, nil
}

// Options locate the directories Run works in.
type Options struct {
	// Root is the repository root.
	Root string
	// OutDir is where outputs are written. It defaults to Root.
	OutDir string
	// DryRun computes the plan without writing anything.
	DryRun bool
	// Packs are the packs adder was built with.
	Packs *pack.Registry
}

// Run plans the derived outputs against the directories in opts and, unless DryRun is set, applies the plan.
func Run(ctx context.Context, cfg *config.Config, opts Options) (*plan.Plan, error) {
	repo, err := os.OpenRoot(opts.Root)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: opts.Root}, err)
	}
	defer repo.Close()

	templates, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: cfg.Dir}, err)
	}
	defer templates.Close()

	outDir := cmp.Or(opts.OutDir, opts.Root)
	out, err := os.OpenRoot(outDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		out = nil
	case err != nil:
		return nil, diag.Wrap(diag.IO, diag.Pos{File: outDir}, err)
	default:
		defer out.Close()
	}

	src := Sources{
		Repo: repo.FS(), RepoDir: opts.Root,
		Templates: templates.FS(), TemplatesDir: cfg.Dir,
		ExistingDir: outDir,
		Packs:       opts.Packs,
	}
	if out != nil {
		src.Existing = out.FS()
	}
	p, err := Plan(ctx, cfg, src)
	if err != nil || opts.DryRun || len(p.Pending()) == 0 {
		return p, err
	}

	if out == nil {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return nil, diag.Wrap(diag.IO, diag.Pos{File: outDir}, err)
		}
		if out, err = os.OpenRoot(outDir); err != nil {
			return nil, diag.Wrap(diag.IO, diag.Pos{File: outDir}, err)
		}
		defer out.Close()
	}
	return p, p.Apply(ctx, out)
}

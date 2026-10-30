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

	// RepoDir, TemplatesDir, and ExistingDir name the directories behind each file system.
	// They only affect the file paths reported in diagnostics and may be empty.
	RepoDir, TemplatesDir, ExistingDir string
}

// Plan computes every derived output and how it differs from the existing files, without writing anything.
// It reports every problem it finds rather than stopping at the first.
func Plan(ctx context.Context, cfg *config.Config, src Sources) (*plan.Plan, error) {
	set, data, err := Prepare(ctx, cfg, src)
	if err != nil {
		return nil, err
	}

	// Single outputs claim their paths first, so an item rendering onto one is reported wherever it appears.
	paths := make(map[string]bool, len(cfg.Derive.Outputs))
	for _, out := range cfg.Derive.Outputs {
		if out.Each == "" {
			paths[out.Path] = true
		}
	}
	var problems diag.List
	var p plan.Plan
	for _, out := range cfg.Derive.Outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if out.Each == "" {
			problems = append(problems, renderOutput(set, data, src, out, out.Path, &p)...)
			continue
		}
		found, err := planEach(ctx, set, data, src, out, &p, paths)
		if err != nil {
			return nil, err
		}
		problems = append(problems, found...)
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return &p, nil
}

// renderOutput adds the output rendered to target to p.
func renderOutput(set *render.Set, data map[string]any, src Sources, out config.Output, target string, p *plan.Plan) diag.List {
	content, err := set.File(out.Template, target, data)
	if err == nil {
		content, err = header.Ensure(target, content, out.Template)
	}
	if err != nil {
		return diag.Locate(err, src.TemplatesDir, out.Pos)
	}
	if err := p.Add(src.Existing, target, content); err != nil {
		return diag.Locate(err, src.ExistingDir, out.Pos)
	}
	if change := p.Changes[len(p.Changes)-1]; change.Status == plan.Updated && !replaceable(change) {
		p.Changes = p.Changes[:len(p.Changes)-1]
		return diag.Locate(&diag.Diagnostic{
			Code:    diag.FileExists,
			Pos:     diag.Pos{File: target},
			Message: fmt.Sprintf("file already exists and adder did not generate it, so it is not replaced by %s", out.Template),
			Hint:    "adder never overwrites files people own; move or delete the file, or write the output somewhere else",
		}, src.ExistingDir, out.Pos)
	}
	return nil
}

// replaceable reports whether derive may overwrite a file with an output. A file whose new content carries
// adder's header must already carry one, naming any template so that templates can be renamed. When the new
// content has none, because the file type has no comments or the template writes its own generated-code
// comment, adder cannot tell its files apart and replaces them as before.
func replaceable(c plan.Change) bool {
	if _, ours := header.Parse(c.Path, c.New); !ours {
		return true
	}
	_, ours := header.Parse(c.Path, c.Old)
	return ours
}

// planEach adds the output rendered for each item of its scan to p, then the files it generated before that it
// no longer produces. Paths holds every path planned so far.
func planEach(ctx context.Context, set *render.Set, data map[string]any, src Sources, out config.Output, p *plan.Plan, paths map[string]bool) (diag.List, error) {
	items, _ := data[out.Each].([]scan.Item)
	produced := make(map[string]bool, len(items))
	var problems diag.List
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		itemData := maps.Clone(data)
		itemData["Item"] = item
		target, err := itemPath(out, item, itemData, paths)
		if err != nil {
			problems = append(problems, diag.From(err)...)
			continue
		}
		paths[target], produced[target] = true, true
		problems = append(problems, renderOutput(set, itemData, src, out, target, p)...)
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
func itemPath(out config.Output, item scan.Item, data map[string]any, paths map[string]bool) (string, error) {
	field := fmt.Sprintf("output %q: path %q", out.Template, out.Path)
	rendered, err := render.Text(out.Path, data)
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
			if name != dir && scan.Ignored(entry.Name()) {
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
		if source, ok := header.Parse(name, content); ok && source == out.Template {
			stale = append(stale, plan.Change{Path: name, Status: plan.Removed, Old: content})
		}
		return nil
	})
	return stale, err
}

// Prepare parses the partials and runs every scan, returning the templates and the data they render with.
func Prepare(ctx context.Context, cfg *config.Config, src Sources) (*render.Set, map[string]any, error) {
	set, err := templates(cfg, src)
	problems := diag.From(err)
	data, err := templateData(ctx, cfg, src)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, nil, err
	}
	problems = append(problems, diag.From(err)...)
	if err := problems.Err(); err != nil {
		return nil, nil, err
	}
	return set, data, nil
}

// templates parses the configured partials into the set every output renders with.
func templates(cfg *config.Config, src Sources) (*render.Set, error) {
	var files []string
	var problems diag.List
	for _, partial := range cfg.Partials {
		matches, err := fs.Glob(src.Templates, partial.Pattern)
		if err != nil {
			problems = append(problems, diag.Wrap(diag.InvalidValue, partial.Pos, err))
			continue
		}
		matches = slices.DeleteFunc(matches, func(name string) bool {
			info, err := fs.Stat(src.Templates, name)
			return err == nil && info.IsDir()
		})
		if len(matches) == 0 {
			problems = append(problems, &diag.Diagnostic{
				Code:    diag.NoMatch,
				Pos:     partial.Pos,
				Message: fmt.Sprintf("partial %q matches no files", partial.Pattern),
				Hint:    "partial patterns are relative to the directory holding adder.yaml",
			})
			continue
		}
		files = append(files, matches...)
	}

	slices.Sort(files)
	set, err := render.NewSet(src.Templates, slices.Compact(files))
	if err != nil {
		problems = append(problems, diag.Locate(err, src.TemplatesDir, diag.Pos{})...)
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return set, nil
}

// templateData runs every scan and adds the vars, and the module path when the repository has a go.mod.
func templateData(ctx context.Context, cfg *config.Config, src Sources) (map[string]any, error) {
	data := make(map[string]any, len(cfg.Derive.Scans)+2)
	vars := cfg.Vars
	if vars == nil {
		vars = map[string]string{}
	}
	data["Vars"] = vars

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
		items, err := scan.Run(ctx, src.Repo, module, s)
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

// Options locate the directories Run works in.
type Options struct {
	// Root is the repository root.
	Root string
	// OutDir is where outputs are written. It defaults to Root.
	OutDir string
	// DryRun computes the plan without writing anything.
	DryRun bool
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

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

	var problems diag.List
	var p plan.Plan
	for _, out := range cfg.Derive.Outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content, err := set.File(out.Template, out.Path, data)
		if err == nil && path.Ext(out.Path) == ".go" {
			content, err = header.Ensure(content, out.Template)
		}
		if err != nil {
			problems = append(problems, diag.Locate(err, src.TemplatesDir, out.Pos)...)
			continue
		}
		if err := p.Add(src.Existing, out.Path, content); err != nil {
			problems = append(problems, diag.Locate(err, src.ExistingDir, out.Pos)...)
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return &p, nil
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

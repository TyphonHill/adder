// Package derive rebuilds files whose content follows from the repository's source code.
package derive

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
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
	data, err := templateData(ctx, cfg, src)
	if err != nil {
		return nil, err
	}

	var p plan.Plan
	var problems diag.List
	for _, out := range cfg.Derive.Outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content, err := render.File(src.Templates, out.Template, out.Path, data)
		if err == nil && path.Ext(out.Path) == ".go" {
			content, err = header.Ensure(content, out.Template)
		}
		if err != nil {
			problems = append(problems, locate(err, src.TemplatesDir, out.Pos)...)
			continue
		}
		if err := p.Add(src.Existing, out.Path, content); err != nil {
			problems = append(problems, locate(err, src.ExistingDir, out.Pos)...)
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return &p, nil
}

// templateData runs every scan and adds the module path when the repository has a go.mod.
func templateData(ctx context.Context, cfg *config.Config, src Sources) (map[string]any, error) {
	data := make(map[string]any, len(cfg.Derive.Scans)+1)
	var problems diag.List
	for _, name := range slices.Sorted(maps.Keys(cfg.Derive.Scans)) {
		s := cfg.Derive.Scans[name]
		items, err := scan.Run(ctx, src.Repo, s)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if err != nil {
			problems = append(problems, locate(err, src.RepoDir, s.Pos)...)
			continue
		}
		data[name] = items
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}

	gomod, err := fs.ReadFile(src.Repo, "go.mod")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return data, nil
	case err != nil:
		return nil, locate(diag.Wrap(diag.IO, diag.Pos{File: "go.mod"}, err), src.RepoDir, diag.Pos{})
	}
	if module := modfile.ModulePath(gomod); module != "" {
		data["Module"] = module
	}
	return data, nil
}

// locate makes diagnostic positions usable from the current directory. Positions inside a file system are
// joined to the directory behind it, and problems without a position are attributed to the declaration at decl.
func locate(err error, dir string, decl diag.Pos) diag.List {
	list := diag.From(err)
	for _, d := range list {
		switch {
		case d.Pos == (diag.Pos{}):
			d.Pos = decl
		case dir != "" && d.Pos.File != "" && !filepath.IsAbs(d.Pos.File):
			d.Pos.File = filepath.Join(dir, filepath.FromSlash(d.Pos.File))
		}
	}
	return list
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

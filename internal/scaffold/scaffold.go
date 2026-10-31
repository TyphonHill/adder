// Package scaffold creates new files from the generators in adder.yaml.
package scaffold

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/pack"
	"typhonhill.dev/adder/internal/plan"
	"typhonhill.dev/adder/internal/render"
)

// Result is what running a generator does.
type Result struct {
	// Plan holds the new files, then the derived outputs.
	Plan *plan.Plan
	// Message is the generator's rendered message, empty when it has none.
	Message string
}

// Plan computes the files the generator called name creates and the derived outputs they change, without
// writing anything. Inputs are by input name: strings for string inputs and booleans for bool inputs.
// New files are created in src.Repo, and derived outputs are compared with src.Existing.
func Plan(ctx context.Context, cfg *config.Config, src derive.Sources, name string, inputs map[string]any) (*Result, error) {
	res, created, err := planFiles(ctx, cfg, src, name, inputs)
	if err != nil {
		return nil, err
	}
	if gen := cfg.Generators[name]; gen.Derive == nil || *gen.Derive {
		withNew := src
		withNew.Repo = overlay{base: src.Repo, files: created}
		withNew.Overlay = created
		derived, err := derive.Plan(ctx, cfg, withNew)
		if err != nil {
			return nil, err
		}
		res.Plan.Changes = append(res.Plan.Changes, derived.Changes...)
	}
	return res, nil
}

// planFiles plans the files the generator called name creates, without the derived outputs, and returns the
// content of the new ones by path.
func planFiles(ctx context.Context, cfg *config.Config, src derive.Sources, name string, inputs map[string]any) (*Result, map[string][]byte, error) {
	gen, ok := cfg.Generators[name]
	if !ok {
		return nil, nil, unknownGenerator(cfg, name)
	}
	set, data, err := derive.Prepare(ctx, cfg, src, gen.Use)
	if err != nil {
		return nil, nil, err
	}
	values, err := resolve(gen, inputs, data)
	if err != nil {
		return nil, nil, err
	}
	data = maps.Clone(data)
	data["Inputs"] = values

	p := &plan.Plan{}
	created := make(map[string][]byte)
	seen := make(map[string]bool)
	var problems diag.List
	for i, f := range gen.Files {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !included(f, values) {
			continue
		}
		change, err := newFile(ctx, cfg, src, gen, set, data, i, f, seen)
		if err != nil {
			problems = append(problems, diag.From(err)...)
			continue
		}
		seen[change.Path] = true
		p.Changes = append(p.Changes, change)
		if change.Status == plan.Created {
			created[change.Path] = change.New
		}
	}

	res := &Result{Plan: p}
	if gen.Message != "" {
		diag.Warn(ctx, diag.Locate(set.TextDeprecations(gen.Message, src.Packs.DeprecatedFuncs()), "", gen.Pos)...)
		msg, err := set.Text(gen.Message, data)
		if err != nil {
			problems = append(problems, diag.Prefix(err, gen.Pos, "message: ")...)
		}
		res.Message = msg
	}
	if err := problems.Err(); err != nil {
		return nil, nil, err
	}
	return res, created, nil
}

// newFile renders the file f of a generator, reporting the deprecated functions and fields in its template and
// path through ctx. Seen holds the paths of the files planned so far.
func newFile(ctx context.Context, cfg *config.Config, src derive.Sources, gen config.Generator, set *render.Set, data map[string]any, i int, f config.File, seen map[string]bool) (plan.Change, error) {
	field := fmt.Sprintf("file %d: path %q", i, f.Path)
	rendered, err := set.Text(f.Path, data)
	if err != nil {
		return plan.Change{}, diag.Prefix(err, f.Pos, field+": ")
	}
	if rendered != f.Path {
		field += fmt.Sprintf(" renders as %q, which", rendered)
	}
	target, d := config.CheckFile(f.Pos, field, rendered)
	switch {
	case d != nil:
		return plan.Change{}, d
	case seen[target]:
		return plan.Change{}, diag.Errorf(diag.DuplicatePath, f.Pos, "%s is also created by another file of the generator", field)
	case slices.ContainsFunc(cfg.Outputs(), func(out config.Output) bool { return out.Path == target }):
		return plan.Change{}, diag.Errorf(diag.DuplicatePath, f.Pos, "%s is a derived output", field)
	}
	if err := plan.CheckLinks(src.Repo, target); err != nil {
		return plan.Change{}, diag.Locate(err, src.RepoDir, f.Pos)
	}

	pack := gen.Pack
	if f.Pack != "" {
		pack = f.Pack
		if set, err = derive.PackTemplates(cfg, src, f.Pack); err != nil {
			return plan.Change{}, diag.InPack(err, pack, f.Pos)
		}
	}
	locate := func(err error) diag.List {
		if pack != "" {
			return diag.InPack(err, pack, f.Pos)
		}
		return diag.Locate(err, src.TemplatesDir, f.Pos)
	}
	content, err := set.File(f.Template, target, data)
	if err != nil {
		return plan.Change{}, locate(err)
	}
	if warnings, err := set.Deprecations(f.Template, src.Packs.DeprecatedFuncs()); err == nil {
		diag.Warn(ctx, locate(warnings)...)
	}
	diag.Warn(ctx, diag.Locate(set.TextDeprecations(f.Path, src.Packs.DeprecatedFuncs()), "", f.Pos)...)
	_, err = fs.Stat(src.Repo, target)
	switch {
	case err == nil && f.IfExists == config.IfExistsSkip:
		return plan.Change{Path: target, Status: plan.Skipped, New: content}, nil
	case err == nil:
		return plan.Change{}, diag.Locate(&diag.Diagnostic{
			Code:    diag.FileExists,
			Pos:     diag.Pos{File: target},
			Message: "file already exists",
			Hint:    "adder never overwrites files people own; choose another name, or set ifExists: skip for files that may already exist",
		}, src.RepoDir, f.Pos)
	case !errors.Is(err, fs.ErrNotExist):
		return plan.Change{}, diag.Locate(diag.Wrap(diag.IO, diag.Pos{File: target}, err), src.RepoDir, f.Pos)
	}
	return plan.Change{Path: target, Status: plan.Created, New: content}, nil
}

// included reports whether a file's when or unless condition holds.
func included(f config.File, values map[string]any) bool {
	switch {
	case f.When != "":
		return values[f.When] == true
	case f.Unless != "":
		return values[f.Unless] != true
	}
	return true
}

func unknownGenerator(cfg *config.Config, name string) error {
	d := diag.Errorf(diag.UnknownGenerator, diag.Pos{}, "unknown generator %q", name)
	if names := slices.Sorted(maps.Keys(cfg.Generators)); len(names) > 0 {
		d.Hint = "generators: " + strings.Join(names, ", ")
	} else {
		d.Hint = "no generators are defined; add them under generators in adder.yaml"
	}
	return d
}

// Options locate the directories Run works in.
type Options struct {
	// Root is the repository root, where new files are created.
	Root string
	// DryRun computes the plan without writing anything.
	DryRun bool
	// Packs are the packs adder was built with.
	Packs *pack.Registry
}

// Run plans the generator called name with inputs and, unless DryRun is set, applies the plan.
func Run(ctx context.Context, cfg *config.Config, name string, inputs map[string]any, opts Options) (*Result, error) {
	src, closeSources, err := derive.Open(cfg, opts.Root, opts.Packs)
	if err != nil {
		return nil, err
	}
	defer closeSources()
	res, err := Plan(ctx, cfg, src, name, inputs)
	if err != nil || opts.DryRun {
		return res, err
	}
	repo, err := os.OpenRoot(opts.Root)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: opts.Root}, err)
	}
	defer repo.Close()
	return res, res.Plan.Apply(ctx, repo)
}

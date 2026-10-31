package derive

import (
	"context"
	"io/fs"
	"maps"
	"slices"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/header"
	"typhonhill.dev/adder/internal/plan"
	"typhonhill.dev/adder/internal/render"
)

// Explanation says where a file comes from, for adder explain.
type Explanation struct {
	// Path is the file's slash-separated path relative to the repository root.
	Path string `json:"path"`
	// Generated reports whether the file has adder's header.
	Generated bool `json:"generated"`
	// Source is what the header says the file was generated from: a template, with its pack and use for a
	// pack's template. It is empty when the file has no header.
	Source string `json:"source"`
	// Output is the output that writes or removes the file, nil when none does.
	Output *config.Output `json:"output"`
	// Scans are the scans the output's template refers to by name, empty when no output writes the file.
	Scans []string `json:"scans"`
	// Status is what derive would do to the file: unchanged, updated, or removed. It is nil when Output is.
	Status *plan.Status `json:"status"`
}

// Explain reports where the file name, a slash-separated path relative to the repository root, comes from: the
// output and template that write it, the scans the template refers to, and whether it is up to date.
func Explain(ctx context.Context, cfg *config.Config, src Sources, name string) (*Explanation, error) {
	content, err := fs.ReadFile(src.Repo, name)
	if err != nil {
		return nil, diag.Locate(diag.Wrap(diag.IO, diag.Pos{File: name}, err), src.RepoDir, diag.Pos{})
	}
	e := &Explanation{Path: name, Scans: []string{}}
	e.Source, e.Generated = header.Parse(name, content)
	if !e.Generated {
		return e, nil
	}
	var output *config.Output
	var owner unit
	for _, u := range units(cfg, src) {
		for i, out := range u.cfg.Derive.Outputs {
			if output == nil && out.Source() == e.Source && writes(out, name) {
				output, owner = &u.cfg.Derive.Outputs[i], u
			}
		}
	}
	if output == nil {
		return e, nil
	}

	// The output only manages the file if the plan writes or removes it: a matching file in a directory that
	// stale cleanup skips, such as testdata, belongs to no output.
	p, err := Plan(ctx, cfg, src)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(p.Changes, func(c plan.Change) bool { return c.Path == name })
	if i < 0 {
		return e, nil
	}
	scanNames := slices.Sorted(maps.Keys(owner.cfg.Derive.Scans))
	if output.Pack != "" && owner.pack == "" {
		owner = packUnit(cfg, output.Pack)
	}
	set, err := templates(owner, src)
	if err != nil {
		return nil, err
	}
	scans, err := set.References(output.Template, scanNames)
	if err != nil {
		return nil, owner.locate(err, output.Pos)
	}
	e.Output, e.Scans, e.Status = output, scans, &p.Changes[i].Status
	return e, nil
}

// writes reports whether out could write the file name: its path, or for outputs with each, a path matching
// its path template.
func writes(out config.Output, name string) bool {
	if out.Each == "" {
		return out.Path == name
	}
	pattern, _, err := render.PathPattern(out.Path)
	return err == nil && pattern.MatchString(name)
}

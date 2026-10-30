package derive

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/pack"
	"github.com/TyphonHill/adder/internal/plan"
)

// routes lists the routes in each directory's routes.txt, one "METHOD /path" per line, as maps.
func routes(_ context.Context, in pack.ScanInput) ([]any, error) {
	var opts struct {
		Prefix string `yaml:"prefix"`
	}
	if err := in.Decode(&opts); err != nil {
		return nil, err
	}
	var items []any
	var problems diag.List
	for _, dir := range in.Dirs {
		file := path.Join(dir, "routes.txt")
		data, err := fs.ReadFile(in.FS, file)
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			method, p, ok := strings.Cut(line, " ")
			if !ok {
				problems = append(problems, &diag.Diagnostic{Code: diag.Scanner, Pos: diag.Pos{File: file, Line: i + 1}, Message: "want METHOD /path"})
				continue
			}
			items = append(items, map[string]any{"method": method, "path": opts.Prefix + p, "name": strings.ToLower(method) + strings.ReplaceAll(p, "/", "_")})
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return items, nil
}

func TestPlanScanners(t *testing.T) {
	type table struct{ Name, Columns string }
	registry, err := pack.New([]pack.Pack{{Name: "local", Scanners: map[string]pack.Scanner{
		"routes": routes,
		"tables": func(context.Context, pack.ScanInput) ([]any, error) {
			return []any{table{"users", "id,name"}, &table{"accounts", "id"}}, nil
		},
		"nothing": func(context.Context, pack.ScanInput) ([]any, error) { return nil, nil },
		"failing": func(context.Context, pack.ScanInput) ([]any, error) { return nil, errors.New("no database") },
	}}})
	if err != nil {
		t.Fatal(err)
	}
	repo := fstest.MapFS{
		"api/a/routes.txt": {Data: []byte("GET /users\nPOST /users\n")},
		"api/b/routes.txt": {Data: []byte("GET /health\n")},
		"api/bad/x.txt":    {Data: []byte("x\n")},
	}
	templates := fstest.MapFS{
		"routes.go.tmpl": {Data: []byte("package api\n\n// {{ range sortBy \"path\" .Routes }}{{ .method }} {{ .path }}, {{ end }}\n")},
		"route.go.tmpl":  {Data: []byte("package api\n\n// {{ .Item.method }}\n")},
		"tables.go.tmpl": {Data: []byte("package db\n\n// {{ fieldValues \"Name\" .Tables | join \", \" }}; {{ len .None }}\n")},
	}
	tests := []struct {
		name        string
		scans       string
		outputs     string
		repo        fstest.MapFS
		wantChanges map[string]string
		wantErrs    []string
	}{
		{
			name:    "map items, with settings, in a single output and one file each",
			scans:   `Routes: {dirs: ["api/a", "api/b"], scanner: {name: routes, with: {prefix: /v1}}}`,
			outputs: `{template: routes.go.tmpl, path: api/routes.gen.go}, {template: route.go.tmpl, each: Routes, path: "api/{{ .Item.name }}.gen.go"}`,
			wantChanges: map[string]string{
				"api/routes.gen.go":     "// GET /v1/health, GET /v1/users, POST /v1/users,",
				"api/post_users.gen.go": "// POST",
				"api/get_health.gen.go": "// GET",
			},
		},
		{
			name:     "two items on one path",
			scans:    `Routes: {dirs: ["api/a"], scanner: {name: routes}}`,
			outputs:  `{template: route.go.tmpl, each: Routes, path: "api/{{ .Item.path }}.gen.go"}`,
			wantErrs: []string{"adder.yaml:5:13 duplicate-path"},
		},
		{
			name:        "struct items and none",
			scans:       "Tables: {scanner: {name: tables}}\n    None: {scanner: {name: nothing}}",
			outputs:     "{template: tables.go.tmpl, path: db/tables.gen.go}",
			wantChanges: map[string]string{"db/tables.gen.go": "// users, accounts; 0"},
		},
		{
			name:     "a problem at a file in the repository",
			scans:    `Routes: {dirs: ["api/*"], scanner: {name: routes}}`,
			outputs:  "{template: routes.go.tmpl, path: api/routes.gen.go}",
			repo:     fstest.MapFS{"api/a/routes.txt": {Data: []byte("GET /a\nbroken\n")}},
			wantErrs: []string{"repo/api/a/routes.txt:2 scanner"},
		},
		{name: "another error", scans: "T: {scanner: {name: failing}}", outputs: "{template: tables.go.tmpl, path: x.go}", wantErrs: []string{"adder.yaml:4:5 scanner"}},
		{name: "settings the scanner rejects", scans: "R: {dirs: [api/a], scanner: {name: routes, with: {prefx: x}}}", outputs: "{template: routes.go.tmpl, path: x.go}", wantErrs: []string{"adder.yaml:4:55 unknown-field"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte("version: 1\nderive:\n  scans:\n    "+tt.scans+"\n  outputs: ["+tt.outputs+"]\n"), registry)
			if err != nil {
				t.Fatal(err)
			}
			r := repo
			if tt.repo != nil {
				r = tt.repo
			}
			p, err := Plan(t.Context(), cfg, Sources{Repo: r, RepoDir: "repo", Templates: templates, Packs: registry})
			var errs []string
			for _, d := range diag.From(err) {
				errs = append(errs, strings.ReplaceAll(d.Pos.String(), `\`, "/")+" "+string(d.Code))
			}
			if !slices.Equal(errs, tt.wantErrs) {
				t.Fatalf("Plan() problems = %q, want %q\n%v", errs, tt.wantErrs, err)
			}
			if err != nil {
				return
			}
			for file, want := range tt.wantChanges {
				i := slices.IndexFunc(p.Changes, func(c plan.Change) bool { return c.Path == file })
				if i < 0 || !strings.Contains(string(p.Changes[i].New), want) {
					t.Errorf("%s = %s, want it to contain %q", file, contentOf(p, i), want)
				}
			}
		})
	}
}

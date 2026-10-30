package derive

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhonhill.dev/adder/internal/config"
)

var budget = flag.Bool("budget", false, "fail when derive is far slower than its performance budget")

// The performance budget for deriving a large repository, from design decision 36. TestBudget fails above
// four times the budget, which catches order-of-magnitude slowdowns without failing on noisy timing.
const (
	largePackages   = 1000
	largeFilesPer   = 2
	largeFuncsPer   = 5 // per file, so 10,000 handlers in all
	budgetTime      = 500 * time.Millisecond
	budgetTolerance = 4
)

func BenchmarkDerive(b *testing.B) {
	cfg, opts := largeRepo(b)
	for b.Loop() {
		if _, err := Run(b.Context(), cfg, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// TestBudget runs derive over a large repository and fails when it is far over budget. It only runs with
// -budget, since timing is meaningless under the race detector or on a busy machine.
func TestBudget(t *testing.T) {
	if !*budget {
		t.Skip("run with -budget to check the performance budget")
	}
	cfg, opts := largeRepo(t)
	best := time.Duration(1<<63 - 1)
	for range 3 {
		start := time.Now()
		p, err := Run(t.Context(), cfg, opts)
		if err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(start))
		if len(p.Changes) != 1 {
			t.Fatalf("Run() planned %d changes, want 1", len(p.Changes))
		}
	}
	t.Logf("derive over %d packages took %v; the budget is %v", largePackages, best, budgetTime)
	if best > budgetTolerance*budgetTime {
		t.Errorf("derive took %v, over %d times the %v budget", best, budgetTolerance, budgetTime)
	}
}

// largeRepo writes a repository of largePackages packages and a registry of every handler in them.
func largeRepo(tb testing.TB) (*config.Config, Options) {
	tb.Helper()
	root := tb.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/large\n\ngo 1.26.0\n",
		"adder.yaml": `version: 1
derive:
  scans:
    Handlers:
      dirs: ["services/**"]
      funcs: {exported: true, name: ^Handle, firstParamType: context.Context}
  outputs:
    - template: registry.go.tmpl
      path: registry/registry.gen.go
`,
		"registry.go.tmpl": `package registry

import (
	"context"
{{ range uniq (fieldValues "ImportPath" .Handlers) }}
	"{{ . }}"
{{- end }}
)

// All lists every handler.
var All = []func(context.Context) error{
{{- range .Handlers }}
	{{ .Package }}.{{ .Name }},
{{- end }}
}
`,
	}
	for p := range largePackages {
		pkg := fmt.Sprintf("p%04d", p)
		for f := range largeFilesPer {
			var src strings.Builder
			fmt.Fprintf(&src, "package %s\n\nimport \"context\"\n", pkg)
			for fn := range largeFuncsPer {
				fmt.Fprintf(&src, "\n// HandleF%dN%d handles requests.\nfunc HandleF%dN%d(ctx context.Context) error { return helper%d() }\n", f, fn, f, fn, f)
			}
			fmt.Fprintf(&src, "\nfunc helper%d() error { return nil }\n", f)
			files[fmt.Sprintf("services/group%d/%s/file%d.go", p%10, pkg, f)] = src.String()
		}
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	cfg, err := config.Load(filepath.Join(root, config.FileName), nil)
	if err != nil {
		tb.Fatal(err)
	}
	return cfg, Options{Root: root, DryRun: true}
}

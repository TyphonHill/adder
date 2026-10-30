package cli

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/plan"
	"github.com/TyphonHill/adder/internal/scaffold"
)

// testReport is what test reports.
type testReport struct {
	Tests []testResultReport `json:"tests"`
	problems
}

type testResultReport struct {
	Generator string         `json:"generator"`
	Name      string         `json:"name"`
	Passed    bool           `json:"passed"`
	Changes   []changeReport `json:"changes"`
}

func newTestCommand(opts *globalOptions) *cobra.Command {
	var update bool
	cmd := &cobra.Command{
		Use:   "test [generator...]",
		Short: "Check generators against their golden files",
		Long: `test runs the tests declared under each generator's tests in adder.yaml, or only those of the generators
named, and compares the files each generator creates with the files under its want directory. It runs
against the repository like --dry-run and writes nothing, unless --update rewrites the want directories.`,
		ValidArgsFunction: completeGenerators(opts, 0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var names, patterns []string
			for _, arg := range args {
				if isPattern(arg) {
					patterns = append(patterns, arg)
				} else {
					names = append(names, arg)
				}
			}
			found := make(map[string]bool)
			newReport := func() report { return &testReport{Tests: []testResultReport{}} }
			err := runConfigs(cmd, opts, patterns, newReport, func(run configRun, r report) error {
				return runTests(cmd, opts, run, r.(*testReport), names, len(patterns) > 0, found, update)
			})
			if len(patterns) > 0 && err == nil {
				for _, name := range names {
					if !found[name] {
						return diag.Errorf(diag.UnknownGenerator, diag.Pos{}, "unknown generator %q: no configuration matched by the patterns has it", name)
					}
				}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "rewrite the want directories with what the generators create now")
	return cmd
}

// runTests runs the tests of the named generators, or of every generator, in one configuration. With several
// configurations, a named generator a configuration lacks is skipped there, and found records the ones seen.
func runTests(cmd *cobra.Command, opts *globalOptions, run configRun, rep *testReport, names []string, several bool, found map[string]bool, update bool) error {
	cfg := run.cfg
	var selected []string
	for _, name := range names {
		switch _, ok := cfg.Generators[name]; {
		case ok:
			selected = append(selected, name)
			found[name] = true
		case !several:
			return unknownGenerator(generatorsCommand(opts, cfg, run.root), cfg, name)
		}
	}
	if len(names) == 0 {
		selected = slices.Sorted(maps.Keys(cfg.Generators))
	}

	src, closeSources, err := opts.open(cfg, run.root)
	if err != nil {
		return err
	}
	defer closeSources()
	var results []*scaffold.TestResult
	for _, name := range slices.Compact(slices.Sorted(slices.Values(selected))) {
		for _, test := range cfg.Generators[name].Tests {
			res, err := scaffold.Test(cmd.Context(), cfg, src, name, test)
			if err != nil {
				return err
			}
			results = append(results, res)
			rep.Tests = append(rep.Tests, testResultReport{
				Generator: name,
				Name:      test.Name,
				Passed:    res.Passed(),
				Changes:   changesReport(res.Changes, true),
			})
		}
	}

	out := opts.printer(cmd.OutOrStdout())
	out.prefix = run.prefix
	if update {
		if err := updateGolden(cmd.Context(), cfg, results); err != nil {
			return err
		}
		if !opts.json {
			printUpdates(out, results)
		}
		return nil
	}
	if !opts.json {
		printTests(out, results)
	}
	var failures diag.List
	for _, res := range results {
		if !res.Passed() {
			failures = append(failures, &diag.Diagnostic{
				Code:    diag.GoldenMismatch,
				Pos:     res.Test.Pos,
				Message: fmt.Sprintf("test %s/%s: the generator's files differ from %s", res.Generator, res.Test.Name, res.Test.Want),
				Hint:    "run adder test --update to accept what the generator creates now, then review the diff",
			})
		}
	}
	return failures.Err()
}

func printTests(p *printer, results []*scaffold.TestResult) {
	if len(results) == 0 {
		p.printf("no generator tests\n")
		return
	}
	for _, res := range results {
		status, color := "ok  ", green
		if !res.Passed() {
			status, color = "FAIL", red
		}
		p.printf("%s  %s\n", p.paint(color, status), path.Join(p.prefix, res.Generator, res.Test.Name))
	}
	if !p.quiet {
		for _, res := range results {
			p.diffs(res.Changes)
		}
	}
}

func printUpdates(p *printer, results []*scaffold.TestResult) {
	updated := false
	for _, res := range results {
		for _, c := range res.Changes {
			p.change(c, false)
			updated = true
		}
	}
	if !updated {
		p.printf("golden files are up to date\n")
	}
}

// updateGolden rewrites the want directories of the tests that failed, inside the configuration's directory.
func updateGolden(ctx context.Context, cfg *config.Config, results []*scaffold.TestResult) error {
	root, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		return diag.Wrap(diag.IO, diag.Pos{File: cfg.Dir}, err)
	}
	defer root.Close()
	var p plan.Plan
	for _, res := range results {
		p.Changes = append(p.Changes, res.Changes...)
	}
	return diag.Locate(p.Apply(ctx, root), cfg.Dir, diag.Pos{}).Err()
}

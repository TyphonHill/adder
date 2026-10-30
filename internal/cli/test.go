package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := &testReport{Tests: []testResultReport{}}
			opts.report = rep
			cfg, root, err := opts.load()
			if err != nil {
				return err
			}
			for _, name := range args {
				if _, ok := cfg.Generators[name]; !ok {
					return unknownGenerator(generatorsCommand(opts, cfg, root), cfg, name)
				}
			}
			names := args
			if len(names) == 0 {
				names = slices.Sorted(maps.Keys(cfg.Generators))
			}

			src, closeSources, err := derive.Open(cfg, root)
			if err != nil {
				return err
			}
			defer closeSources()
			var results []*scaffold.TestResult
			for _, name := range slices.Compact(slices.Sorted(slices.Values(names))) {
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

			if update {
				if err := updateGolden(cmd.Context(), cfg, results); err != nil {
					return err
				}
				if !opts.json {
					printUpdates(cmd.OutOrStdout(), results)
				}
				return nil
			}
			if !opts.json {
				printTests(cmd.OutOrStdout(), results)
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
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "rewrite the want directories with what the generators create now")
	return cmd
}

func printTests(w io.Writer, results []*scaffold.TestResult) {
	if len(results) == 0 {
		fmt.Fprintln(w, "no generator tests")
		return
	}
	for _, res := range results {
		status := "ok"
		if !res.Passed() {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%-4s  %s/%s\n", status, res.Generator, res.Test.Name)
	}
	for _, res := range results {
		printDiffs(w, res.Changes)
	}
}

func printUpdates(w io.Writer, results []*scaffold.TestResult) {
	updated := false
	for _, res := range results {
		for _, c := range res.Changes {
			fmt.Fprintf(w, "%-9s  %s\n", c.Status, c.Path)
			updated = true
		}
	}
	if !updated {
		fmt.Fprintln(w, "golden files are up to date")
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

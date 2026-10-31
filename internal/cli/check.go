package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"typhonhill.dev/adder/internal/derive"
)

// staleError reports derived outputs that do not match what derive would write.
type staleError struct {
	count int
}

func (e *staleError) Error() string {
	if e.count == 1 {
		return "1 derived file is out of date; run 'adder derive' to update it"
	}
	return fmt.Sprintf("%d derived files are out of date; run 'adder derive' to update them", e.count)
}

func newCheckCommand(opts *globalOptions) *cobra.Command {
	var showDiff bool
	cmd := &cobra.Command{
		Use:   "check [directories]",
		Short: "Report derived files that are out of date",
		Long: `check computes what derive would write and reports every output that differs, without writing anything.

It exits with status 3 when any output is out of date, so it can guard CI and repositories that commit generated files.
With directory patterns, such as ./... or ./services/api, it runs over every adder.yaml they match.`,
		Args: patternArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			newReport := func() report { return &planReport{Changes: []changeReport{}} }
			return runConfigs(cmd, opts, args, newReport, func(run configRun, r report) error {
				rep := r.(*planReport)
				p, err := derive.Run(cmd.Context(), run.cfg, derive.Options{Root: run.root, DryRun: true, Packs: opts.packs})
				if err != nil {
					return err
				}
				rep.Changes = changesReport(p.Changes, showDiff)
				pending := p.Pending()
				if !opts.json {
					out := opts.printer(cmd.OutOrStdout())
					out.prefix = run.prefix
					if err := out.scans(cmd.Context(), run.cfg, run.root); err != nil {
						return err
					}
					for _, c := range p.Changes {
						out.change(c, true)
					}
					if showDiff {
						out.diffs(pending)
					}
				}
				if len(pending) > 0 {
					return &staleError{count: len(pending)}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each out-of-date output as a unified diff")
	return cmd
}

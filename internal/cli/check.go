package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/derive"
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
		Use:   "check",
		Short: "Report derived files that are out of date",
		Long: `check computes what derive would write and reports every output that differs, without writing anything.

It exits with status 3 when any output is out of date, so it can guard CI and repositories that commit generated files.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, root, err := opts.load()
			if err != nil {
				return err
			}
			p, err := derive.Run(cmd.Context(), cfg, derive.Options{Root: root, DryRun: true})
			if err != nil {
				return err
			}
			pending := p.Pending()
			for _, c := range pending {
				fmt.Fprintf(cmd.OutOrStdout(), "would be %-9s  %s\n", c.Status, c.Path)
			}
			if showDiff {
				printDiffs(cmd.OutOrStdout(), pending)
			}
			if len(pending) > 0 {
				return &staleError{count: len(pending)}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each out-of-date output as a unified diff")
	return cmd
}

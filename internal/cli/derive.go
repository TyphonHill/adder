package cli

import (
	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/derive"
)

func newDeriveCommand(opts *globalOptions) *cobra.Command {
	var derived derive.Options
	var showDiff bool
	cmd := &cobra.Command{
		Use:   "derive",
		Short: "Rebuild files derived from the repository's source code",
		Long: `derive runs every scan in adder.yaml and re-renders the derived outputs.

Files whose content has not changed are left untouched.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			derived.DryRun = derived.DryRun || showDiff
			rep := &deriveReport{DryRun: derived.DryRun, Changes: []changeReport{}}
			opts.report = rep
			cfg, root, err := opts.load()
			if err != nil {
				return err
			}
			derived.Root = root
			p, err := derive.Run(cmd.Context(), cfg, derived)
			if err != nil {
				return err
			}
			rep.Changes = changesReport(p.Changes, showDiff)
			if opts.json {
				return nil
			}
			out := opts.printer(cmd.OutOrStdout())
			if err := out.scans(cmd.Context(), cfg, root); err != nil {
				return err
			}
			for _, c := range p.Changes {
				out.change(c, derived.DryRun)
			}
			if showDiff {
				out.diffs(p.Pending())
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&derived.OutDir, "out", "", "write outputs under this directory instead of --root")
	cmd.Flags().BoolVarP(&derived.DryRun, "dry-run", "n", false, "show what would change without writing anything")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each change as a unified diff; implies --dry-run")
	return cmd
}

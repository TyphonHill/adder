package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"typhonhill.dev/adder/internal/derive"
)

func newDeriveCommand(opts *globalOptions) *cobra.Command {
	var derived derive.Options
	var showDiff bool
	cmd := &cobra.Command{
		Use:   "derive",
		Short: "Rebuild files derived from the repository's source code",
		Long: `derive runs every scan in adder.yaml and re-renders the derived outputs.

Files whose content has not changed are left untouched.`,
		Args: patternArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			derived.DryRun = derived.DryRun || showDiff
			if derived.OutDir != "" && len(args) > 0 {
				return &usageError{errors.New("--out writes one configuration's outputs and cannot be combined with directory patterns"), cmd.CommandPath()}
			}
			newReport := func() report { return &deriveReport{DryRun: derived.DryRun, Changes: []changeReport{}} }
			return runConfigs(cmd, opts, args, newReport, func(run configRun, r report) error {
				rep := r.(*deriveReport)
				options := derived
				options.Root = run.root
				options.Packs = opts.packs
				p, err := derive.Run(cmd.Context(), run.cfg, options)
				if err != nil {
					return err
				}
				rep.Changes = changesReport(p.Changes, showDiff)
				if opts.json {
					return nil
				}
				out := opts.printer(cmd.OutOrStdout())
				out.prefix = run.prefix
				if err := out.scans(cmd.Context(), run.cfg, run.root); err != nil {
					return err
				}
				for _, c := range p.Changes {
					out.change(c, derived.DryRun)
				}
				if showDiff {
					out.diffs(p.Pending())
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&derived.OutDir, "out", "", "write outputs under this directory instead of --root")
	cmd.Flags().BoolVarP(&derived.DryRun, "dry-run", "n", false, "show what would change without writing anything")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each change as a unified diff; implies --dry-run")
	return cmd
}

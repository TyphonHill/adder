package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/plan"
)

func newDeriveCommand(opts *globalOptions) *cobra.Command {
	var derived derive.Options
	cmd := &cobra.Command{
		Use:   "derive",
		Short: "Rebuild files derived from the repository's source code",
		Long: `derive runs every scan in adder.yaml and re-renders the derived outputs.

Files whose content has not changed are left untouched.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(opts.configPath())
			if err != nil {
				return err
			}
			derived.Root = opts.root
			p, err := derive.Run(cmd.Context(), cfg, derived)
			if err != nil {
				return err
			}
			for _, c := range p.Changes {
				verb := ""
				if derived.DryRun && c.Status != plan.Unchanged {
					verb = "would be "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%-9s  %s\n", verb, c.Status, c.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&derived.OutDir, "out", "", "write outputs under this directory instead of --root")
	cmd.Flags().BoolVarP(&derived.DryRun, "dry-run", "n", false, "show what would change without writing anything")
	return cmd
}

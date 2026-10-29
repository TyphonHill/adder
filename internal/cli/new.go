package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/plan"
	"github.com/TyphonHill/adder/internal/scaffold"
)

const newLong = `new creates files from a generator declared under generators in adder.yaml.

Each generator is a subcommand with a flag for each of its inputs. Run 'adder new' to list the
generators and 'adder new <generator> --help' to see its inputs. Existing files are never overwritten,
and derived outputs are updated in the same run.`

func newNewCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "new <generator> [flags]",
		Short: "Create files with a generator from adder.yaml",
		Long:  newLong,
		// Generators define their own flags, so the command line is parsed once adder.yaml is loaded.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd, opts, args)
		},
	}
}

func runNew(cmd *cobra.Command, opts *globalOptions, args []string) error {
	pre := pflag.NewFlagSet("new", pflag.ContinueOnError)
	pre.ParseErrorsAllowlist.UnknownFlags = true
	pre.SetOutput(io.Discard)
	pre.StringVar(&opts.root, "root", opts.root, "")
	pre.StringVarP(&opts.config, "config", "c", opts.config, "")
	help := pre.BoolP("help", "h", false, "")
	_ = pre.Parse(args) // mistakes are reported when the generator's command parses the same arguments

	cfg, err := config.Load(opts.configPath())
	if err != nil {
		if *help {
			return cmd.Help()
		}
		return err
	}

	root := &cobra.Command{Use: "adder", SilenceErrors: true, SilenceUsage: true}
	root.CompletionOptions.DisableDefaultCmd = true
	addGlobalFlags(root, opts)
	root.AddCommand(generatorsCommand(cfg, opts))
	root.SetArgs(append([]string{"new"}, args...))
	root.SetIn(cmd.InOrStdin())
	root.SetOut(cmd.OutOrStdout())
	root.SetErr(cmd.ErrOrStderr())
	return root.ExecuteContext(cmd.Context())
}

// generatorsCommand returns the new command with a subcommand for each generator in cfg.
func generatorsCommand(cfg *config.Config, opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <generator> [flags]",
		Short: "Create files with a generator from adder.yaml",
		Long:  newLong,
		// Flags of a mistyped generator should not hide that the generator is unknown.
		FParseErrWhitelist:         cobra.FParseErrWhitelist{UnknownFlags: true},
		SuggestionsMinimumDistance: 2,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return unknownGenerator(cmd, cfg, args[0])
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Generators)) {
		cmd.AddCommand(generatorCommand(cfg, opts, name))
	}
	return cmd
}

func unknownGenerator(cmd *cobra.Command, cfg *config.Config, name string) error {
	d := diag.Errorf(diag.UnknownGenerator, diag.Pos{}, "unknown generator %q", name)
	switch suggestions := cmd.SuggestionsFor(name); {
	case len(suggestions) > 0:
		d.Hint = "did you mean " + strings.Join(suggestions, " or ") + "?"
	case len(cfg.Generators) > 0:
		d.Hint = "generators: " + strings.Join(slices.Sorted(maps.Keys(cfg.Generators)), ", ")
	default:
		d.Hint = "no generators are defined; add them under generators in adder.yaml"
	}
	return d
}

func generatorCommand(cfg *config.Config, opts *globalOptions, name string) *cobra.Command {
	gen := cfg.Generators[name]
	var dryRun, showDiff bool
	var inputsFile string
	cmd := &cobra.Command{
		Use:   name,
		Short: gen.Description,
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			inputs, err := readInputs(cmd, inputsFile)
			if err != nil {
				return err
			}
			for input, in := range gen.Inputs {
				flag := cmd.Flags().Lookup(config.Flag(input))
				switch {
				case !flag.Changed:
				case in.Type == config.InputBool:
					inputs[input] = flag.Value.String() == "true"
				default:
					inputs[input] = flag.Value.String()
				}
			}

			dryRun = dryRun || showDiff
			res, err := scaffold.Run(cmd.Context(), cfg, name, inputs, scaffold.Options{Root: opts.root, DryRun: dryRun})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, c := range res.Plan.Changes {
				switch {
				case c.Status == plan.Unchanged:
					continue
				case dryRun && c.Status != plan.Skipped:
					fmt.Fprintf(out, "would be %-9s  %s\n", c.Status, c.Path)
				default:
					fmt.Fprintf(out, "%-9s  %s\n", c.Status, c.Path)
				}
			}
			if showDiff {
				for _, c := range res.Plan.Pending() {
					fmt.Fprintf(out, "\n%s", c.Diff())
				}
			}
			if msg := strings.TrimSpace(res.Message); msg != "" && !dryRun {
				fmt.Fprintf(out, "\n%s\n", msg)
			}
			return nil
		},
	}

	for _, input := range slices.Sorted(maps.Keys(gen.Inputs)) {
		in := gen.Inputs[input]
		flag := config.Flag(input)
		switch {
		case in.Type == config.InputBool:
			cmd.Flags().Bool(flag, false, in.Help)
		case in.Default != nil:
			cmd.Flags().String(flag, *in.Default, flagHelp(in))
		default:
			cmd.Flags().String(flag, "", flagHelp(in))
		}
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing anything")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each change as a unified diff; implies --dry-run")
	cmd.Flags().StringVar(&inputsFile, "inputs", "", "read inputs from a JSON object in this file, or - for standard input; flags override it")
	return cmd
}

// flagHelp describes a string input for its flag's help.
func flagHelp(in config.Input) string {
	var notes []string
	if in.Required() {
		notes = append(notes, "required")
	}
	switch {
	case in.Values != nil:
		notes = append(notes, "one of: "+strings.Join(in.Values, ", "))
	case in.ValuesFrom != "":
		notes = append(notes, "one of the "+in.ValuesFrom+" values")
	}
	if in.Unique != "" {
		notes = append(notes, "not already in "+in.Unique)
	}
	if len(notes) == 0 {
		return in.Help
	}
	return strings.TrimSpace(fmt.Sprintf("%s (%s)", in.Help, strings.Join(notes, "; ")))
}

// readInputs reads the JSON object of inputs in file, which is - for standard input. No file means no inputs.
func readInputs(cmd *cobra.Command, file string) (map[string]any, error) {
	inputs := make(map[string]any)
	if file == "" {
		return inputs, nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(file)
	}
	pos := diag.Pos{File: file}
	if file == "-" {
		pos = diag.Pos{} // standard input has no name to report
	}
	if err != nil {
		return nil, diag.Wrap(diag.IO, pos, err)
	}
	if err := json.Unmarshal(data, &inputs); err != nil || inputs == nil {
		return nil, &diag.Diagnostic{
			Code:    diag.InvalidInput,
			Pos:     pos,
			Message: "inputs must be a JSON object of input names and values",
			Hint:    `such as {"name": "gcs", "cache": true}`,
			Err:     err,
		}
	}
	return inputs, nil
}

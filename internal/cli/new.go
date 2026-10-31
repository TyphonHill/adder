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

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/scaffold"
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
		ValidArgsFunction:  completeNew(opts),
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

	cfg, root, err := opts.load()
	if err != nil {
		if *help {
			return cmd.Help()
		}
		return err
	}

	tree := &cobra.Command{Use: "adder", SilenceErrors: true, SilenceUsage: true}
	tree.CompletionOptions.DisableDefaultCmd = true
	addGlobalFlags(tree, opts)
	tree.AddCommand(generatorsCommand(opts, cfg, root))
	tree.SetArgs(append([]string{"new"}, args...))
	tree.SetIn(cmd.InOrStdin())
	tree.SetOut(cmd.OutOrStdout())
	tree.SetErr(cmd.ErrOrStderr())
	return tree.ExecuteContext(cmd.Context())
}

// generatorsCommand returns the new command with a subcommand for each generator in cfg, working in root.
func generatorsCommand(opts *globalOptions, cfg *config.Config, root string) *cobra.Command {
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
		cmd.AddCommand(generatorCommand(opts, cfg, root, name))
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

func generatorCommand(opts *globalOptions, cfg *config.Config, root, name string) *cobra.Command {
	gen := cfg.Generators[name]
	var dryRun, showDiff bool
	var inputsFile string
	short := gen.Description
	if gen.Deprecated != "" {
		short = strings.TrimSpace(short + " (deprecated: " + gen.Deprecated + ")")
	}
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun = dryRun || showDiff
			rep := &newReport{Generator: name, DryRun: dryRun, Changes: []changeReport{}}
			opts.report = rep
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
			diag.Warn(cmd.Context(), generatorWarnings(name, gen, inputs)...)
			if !opts.json && inputsFile != "-" && isTerminal(cmd.InOrStdin()) && isTerminal(cmd.ErrOrStderr()) {
				if err := promptMissing(cmd, opts, cfg, root, name, inputs); err != nil {
					return err
				}
			}

			res, err := scaffold.Run(cmd.Context(), cfg, name, inputs, scaffold.Options{Root: root, DryRun: dryRun, Packs: opts.packs})
			if err != nil {
				return err
			}
			rep.Changes = changesReport(res.Plan.Changes, showDiff)
			rep.Message = strings.TrimSpace(res.Message)
			if opts.json {
				return nil
			}
			out := opts.printer(cmd.OutOrStdout())
			for _, c := range res.Plan.Changes {
				out.change(c, dryRun)
			}
			if showDiff {
				out.diffs(res.Plan.Pending())
			}
			if rep.Message != "" && !dryRun {
				out.printf("\n%s\n", rep.Message)
			}
			return nil
		},
	}

	for _, input := range slices.Sorted(maps.Keys(gen.Inputs)) {
		in := gen.Inputs[input]
		flag := config.Flag(input)
		switch {
		case in.Type == config.InputBool:
			cmd.Flags().Bool(flag, false, flagHelp(in))
		case in.Default != nil:
			cmd.Flags().String(flag, *in.Default, flagHelp(in))
		default:
			cmd.Flags().String(flag, "", flagHelp(in))
		}
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would change without writing anything")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "also show each change as a unified diff; implies --dry-run")
	cmd.Flags().StringVar(&inputsFile, "inputs", "", "read inputs from a JSON object in this file, or - for standard input; flags override it")
	_ = cmd.MarkFlagFilename("inputs", "json")
	return cmd
}

// generatorWarnings returns a warning when the generator called name is deprecated, and one for each deprecated
// input among inputs.
func generatorWarnings(name string, gen config.Generator, inputs map[string]any) diag.List {
	var warnings diag.List
	if gen.Deprecated != "" {
		warnings = append(warnings, &diag.Diagnostic{Code: diag.Deprecated, Pos: gen.Pos, Message: fmt.Sprintf("generator %q is deprecated: %s", name, gen.Deprecated)})
	}
	for _, input := range slices.Sorted(maps.Keys(inputs)) {
		if in, ok := gen.Inputs[input]; ok && in.Deprecated != "" {
			warnings = append(warnings, &diag.Diagnostic{Code: diag.Deprecated, Pos: in.Pos, Message: fmt.Sprintf("generator %q: input %q is deprecated: %s", name, input, in.Deprecated)})
		}
	}
	return warnings
}

// flagHelp describes an input for its flag's help.
func flagHelp(in config.Input) string {
	var notes []string
	if in.Deprecated != "" {
		notes = append(notes, "deprecated: "+in.Deprecated)
	}
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

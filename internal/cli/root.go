// Package cli implements the adder command line interface.
package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/pack"
)

// Exit codes returned by Main.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
	ExitStale = 3
)

// usageError marks mistakes in how adder was invoked, such as an unknown flag.
type usageError struct {
	err error
	cmd string // the command whose help explains the usage, such as "adder new driver"
}

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// command returns the command whose help explains the usage.
func (e *usageError) command() string { return cmp.Or(e.cmd, "adder") }

// usageArgs reports argument validation failures as usage errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return &usageError{err, cmd.CommandPath()}
		}
		return nil
	}
}

// globalOptions holds the flags shared by every subcommand, and the report of the command that ran. Empty means
// the flag was not given.
type globalOptions struct {
	root    string
	config  string
	json    bool
	quiet   bool
	verbose bool

	packs  *pack.Registry
	report report // set by a command when it starts running, for --json
	// warnings collects the warnings that concern no one configuration, or the only one. configWarnings are those
	// of each configuration a command with directory patterns ran over, which their reports carry.
	warnings       *diag.Warnings
	configWarnings diag.List
}

// open returns the sources for a configuration with the packs adder was built with.
func (o *globalOptions) open(cfg *config.Config, root string) (derive.Sources, func(), error) {
	return derive.Open(cfg, root, o.packs)
}

// load finds and loads the configuration, returning it with the repository root. The configuration is
// <root>/adder.yaml with --root, the root is the configuration's directory with --config, and without either
// adder.yaml is found by walking up from the current directory.
func (o *globalOptions) load() (*config.Config, string, error) {
	return o.loadFrom(".")
}

// loadFrom is load, finding adder.yaml from dir rather than the current directory.
func (o *globalOptions) loadFrom(dir string) (*config.Config, string, error) {
	root, path := o.root, o.config
	switch {
	case path == "" && root == "":
		found, err := config.Find(dir)
		if err != nil {
			return nil, "", err
		}
		root, path = filepath.Dir(found), found
	case path == "":
		path = filepath.Join(root, config.FileName)
	case root == "":
		root = filepath.Dir(path)
	}
	cfg, err := config.Load(path, o.packs)
	if cfg != nil && o.warnings != nil {
		o.warnings.Add(cfg.Warnings...)
	}
	return cfg, root, err
}

// Main runs adder built with packs on args, reading stdin and writing to stdout and stderr, and returns the
// process exit code.
func Main(ctx context.Context, version string, packs []pack.Pack, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd, opts := newRoot(version)
	opts.json = wantsJSON(args)
	opts.warnings = &diag.Warnings{}
	ctx = diag.WithWarnings(ctx, opts.warnings)
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	registry, err := pack.New(packs)
	var ran *cobra.Command
	if err == nil {
		opts.packs = registry
		ran, err = cmd.ExecuteContextC(ctx)
	}
	switch {
	case opts.json:
		writeReport(stdout, opts.report, opts.warnings.List(), err)
	case completing(ran):
		// Warnings would interrupt the shell's completions.
	default:
		printWarnings(stderr, append(opts.warnings.List(), opts.configWarnings...), useColor(stderr))
		if err != nil {
			printError(stderr, err, useColor(stderr))
		}
	}
	if err == nil {
		return ExitOK
	}
	if many, ok := errors.AsType[*configsError](err); ok {
		switch {
		case many.usage:
			return ExitUsage
		case many.failed:
			return ExitError
		}
		return ExitStale
	}
	if _, ok := errors.AsType[*usageError](err); ok || invocationProblem(err) {
		return ExitUsage
	}
	if _, ok := errors.AsType[*staleError](err); ok {
		return ExitStale
	}
	return ExitError
}

// invocationProblem reports whether every problem in err is a mistake in how a generator was invoked.
func invocationProblem(err error) bool {
	for _, d := range diag.From(err) {
		switch d.Code {
		case diag.UnknownGenerator, diag.MissingInput, diag.InvalidInput:
		default:
			return false
		}
	}
	return true
}

// newRoot returns the root adder command and the options its commands share.
func newRoot(version string) (*cobra.Command, *globalOptions) {
	opts := &globalOptions{}
	cmd := &cobra.Command{
		Use:   "adder",
		Short: "Go boilerplate that maintains itself.",
		Long: `adder generates and maintains the repeated pieces of a Go repository.

Generators and derived files are declared in adder.yaml at the repository root.`,
		Version:       version,
		Args:          usageArgs(cobra.NoArgs),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	addGlobalFlags(cmd, opts)
	cmd.AddCommand(newInitCommand(opts), newDeriveCommand(opts), newCheckCommand(opts), newNewCommand(opts),
		newListCommand(opts), newDescribeCommand(opts), newExplainCommand(opts), newTestCommand(opts))
	markDeprecated(cmd)
	return cmd, opts
}

// deprecatedCommands and deprecatedFlags say what to use instead of each deprecated command, by its path such as
// "adder list", and each deprecated flag, by its command's path and its name such as "adder derive --out". They
// are empty until one is deprecated, as docs/compatibility.md describes, and tests add to them.
var (
	deprecatedCommands = map[string]string{}
	deprecatedFlags    = map[string]string{}
)

// markDeprecated notes in the help of cmd and the commands below it which are deprecated, and which flags.
func markDeprecated(cmd *cobra.Command) {
	if instead, ok := deprecatedCommands[cmd.CommandPath()]; ok {
		cmd.Short += " (deprecated: " + instead + ")"
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if instead, ok := deprecatedFlags[cmd.CommandPath()+" --"+f.Name]; ok {
			f.Usage += " (deprecated: " + instead + ")"
		}
	})
	for _, sub := range cmd.Commands() {
		markDeprecated(sub)
	}
}

// warnDeprecated reports, through the context of cmd, whether cmd or a flag it was given is deprecated.
func warnDeprecated(cmd *cobra.Command) {
	var warnings diag.List
	if instead, ok := deprecatedCommands[cmd.CommandPath()]; ok {
		warnings = append(warnings, &diag.Diagnostic{Code: diag.Deprecated, Message: fmt.Sprintf("command %q is deprecated: %s", cmd.CommandPath(), instead)})
	}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if instead, ok := deprecatedFlags[cmd.CommandPath()+" --"+f.Name]; ok {
			warnings = append(warnings, &diag.Diagnostic{Code: diag.Deprecated, Message: fmt.Sprintf("flag --%s of %q is deprecated: %s", f.Name, cmd.CommandPath(), instead)})
		}
	})
	diag.Warn(cmd.Context(), warnings...)
}

// completing reports whether the command that ran is the hidden one through which cobra answers for shell
// completions.
func completing(ran *cobra.Command) bool {
	return ran != nil && (ran.Name() == cobra.ShellCompRequestCmd || ran.Name() == cobra.ShellCompNoDescRequestCmd)
}

// addGlobalFlags defines the flags every subcommand shares and reports flag mistakes as usage errors.
func addGlobalFlags(cmd *cobra.Command, opts *globalOptions) {
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &usageError{err, c.CommandPath()}
	})
	cmd.PersistentFlags().StringVar(&opts.root, "root", "", "repository root that scans and outputs are relative to (default: the directory holding "+config.FileName+", found from the current directory)")
	cmd.PersistentFlags().StringVarP(&opts.config, "config", "c", "", "configuration file (default <root>/"+config.FileName+")")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", opts.json, "print the result and any problems as one JSON object on standard output")
	cmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", opts.quiet, "print only problems")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", opts.verbose, "also print unchanged files and how many items each scan found")
	_ = cmd.MarkPersistentFlagDirname("root")
	_ = cmd.MarkPersistentFlagFilename("config", "yaml", "yml")
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		if opts.quiet && opts.verbose {
			return &usageError{errors.New("--quiet and --verbose cannot be combined"), c.CommandPath()}
		}
		warnDeprecated(c)
		return nil
	}
}

// printWarnings writes warnings for a person to read, one line per warning, as printError writes problems.
func printWarnings(w io.Writer, warnings diag.List, color bool) {
	p := &printer{w: w, color: color}
	for _, d := range warnings {
		prefix := "adder: "
		if pos := d.Pos.String(); pos != "" {
			prefix = pos + ": "
		}
		fmt.Fprintf(w, "%s%s %s %s\n", prefix, p.paint(yellow, "warning:"), d.Message, p.paint(yellow, "["+string(d.Code)+"]"))
		if d.Hint != "" {
			fmt.Fprintf(w, "\t%s %s\n", p.paint(dim, "hint:"), d.Hint)
		}
	}
}

// printError writes err for a person to read, one line per problem, with codes in color when color is set.
func printError(w io.Writer, err error, color bool) {
	p := &printer{w: w, color: color}
	if usage, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprintf(w, "adder: %v\nRun '%s --help' for usage.\n", usage.err, usage.command())
		return
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, "adder: interrupted")
		return
	}
	if stale, ok := errors.AsType[*staleError](err); ok {
		fmt.Fprintf(w, "adder: %v\n", stale)
		return
	}
	if many, ok := errors.AsType[*configsError](err); ok {
		if len(many.problems) > 0 {
			printError(w, many.problems, color)
		}
		if many.stale > 0 {
			printError(w, &staleError{count: many.stale}, color)
		}
		return
	}
	for _, d := range diag.From(err) {
		prefix := "adder: "
		if d.Pos.File != "" {
			prefix = ""
		}
		fmt.Fprintf(w, "%s%s %s\n", prefix, d.Error(), p.paint(red, "["+string(d.Code)+"]"))
		if d.Hint != "" {
			fmt.Fprintf(w, "\t%s %s\n", p.paint(dim, "hint:"), d.Hint)
		}
	}
}

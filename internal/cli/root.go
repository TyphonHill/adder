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

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/plan"
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
	root   string
	config string
	json   bool

	report report // set by a command when it starts running, for --json
}

// load finds and loads the configuration, returning it with the repository root. The configuration is
// <root>/adder.yaml with --root, the root is the configuration's directory with --config, and without either
// adder.yaml is found by walking up from the current directory.
func (o *globalOptions) load() (*config.Config, string, error) {
	root, path := o.root, o.config
	switch {
	case path == "" && root == "":
		found, err := config.Find(".")
		if err != nil {
			return nil, "", err
		}
		root, path = filepath.Dir(found), found
	case path == "":
		path = filepath.Join(root, config.FileName)
	case root == "":
		root = filepath.Dir(path)
	}
	cfg, err := config.Load(path)
	return cfg, root, err
}

// Main runs adder with args, reading stdin and writing to stdout and stderr, and returns the process exit code.
func Main(ctx context.Context, version string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd, opts := newRoot(version)
	opts.json = wantsJSON(args)
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.ExecuteContext(ctx)
	if opts.json {
		writeReport(stdout, opts.report, err)
	} else if err != nil {
		printError(stderr, err)
	}
	if err == nil {
		return ExitOK
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
		Short: "Write the pattern once. adder adds the rest.",
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
	cmd.AddCommand(newInitCommand(opts), newDeriveCommand(opts), newCheckCommand(opts), newNewCommand(opts))
	return cmd, opts
}

// addGlobalFlags defines the flags every subcommand shares and reports flag mistakes as usage errors.
func addGlobalFlags(cmd *cobra.Command, opts *globalOptions) {
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &usageError{err, c.CommandPath()}
	})
	cmd.PersistentFlags().StringVar(&opts.root, "root", "", "repository root that scans and outputs are relative to (default: the directory holding "+config.FileName+", found from the current directory)")
	cmd.PersistentFlags().StringVarP(&opts.config, "config", "c", "", "configuration file (default <root>/"+config.FileName+")")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", opts.json, "print the result and any problems as one JSON object on standard output")
}

// printDiffs writes each pending change as a unified diff, after a blank line.
func printDiffs(w io.Writer, changes []plan.Change) {
	for _, c := range changes {
		fmt.Fprintf(w, "\n%s", c.Diff())
	}
}

// printError writes err for a person to read, one line per problem.
func printError(w io.Writer, err error) {
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
	for _, d := range diag.From(err) {
		prefix := "adder: "
		if d.Pos.File != "" {
			prefix = ""
		}
		fmt.Fprintf(w, "%s%s [%s]\n", prefix, d.Error(), d.Code)
		if d.Hint != "" {
			fmt.Fprintf(w, "\thint: %s\n", d.Hint)
		}
	}
}

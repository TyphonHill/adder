// Package cli implements the adder command line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
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
}

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// usageArgs reports argument validation failures as usage errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return &usageError{err}
		}
		return nil
	}
}

// globalOptions holds the flags shared by every subcommand.
type globalOptions struct {
	root   string
	config string
}

func (o *globalOptions) configPath() string {
	if o.config != "" {
		return o.config
	}
	return filepath.Join(o.root, config.FileName)
}

// Main runs adder with args, writing to stdout and stderr, and returns the process exit code.
func Main(ctx context.Context, version string, args []string, stdout, stderr io.Writer) int {
	cmd := New(version)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	printError(stderr, err)
	if _, ok := errors.AsType[*usageError](err); ok {
		return ExitUsage
	}
	if _, ok := errors.AsType[*staleError](err); ok {
		return ExitStale
	}
	return ExitError
}

// New returns the root adder command.
func New(version string) *cobra.Command {
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
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{err}
	})
	cmd.PersistentFlags().StringVar(&opts.root, "root", ".", "repository root that scans and outputs are relative to")
	cmd.PersistentFlags().StringVarP(&opts.config, "config", "c", "", "configuration file (default <root>/"+config.FileName+")")

	cmd.AddCommand(newDeriveCommand(opts), newCheckCommand(opts))
	return cmd
}

// printError writes err for a person to read, one line per problem.
func printError(w io.Writer, err error) {
	if usage, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprintf(w, "adder: %v\nRun 'adder --help' for usage.\n", usage.err)
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

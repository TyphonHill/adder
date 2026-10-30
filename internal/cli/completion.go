package cli

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/plan"
	"github.com/TyphonHill/adder/internal/scaffold"
)

// Completion reads adder.yaml and may run scans, but never writes, and completes nothing rather than printing
// errors into the shell when the configuration does not load.

const noFiles = cobra.ShellCompDirectiveNoFileComp

// completeGenerators completes generator names that args does not already hold, with their descriptions.
func completeGenerators(opts *globalOptions, limit int) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if limit > 0 && len(args) >= limit {
			return nil, noFiles
		}
		cfg, _, err := opts.load()
		if err != nil {
			return nil, noFiles
		}
		return generatorNames(cfg, args, toComplete), noFiles
	}
}

func generatorNames(cfg *config.Config, exclude []string, prefix string) []cobra.Completion {
	var names []cobra.Completion
	for _, name := range slices.Sorted(maps.Keys(cfg.Generators)) {
		if strings.HasPrefix(name, prefix) && !slices.Contains(exclude, name) {
			names = append(names, cobra.CompletionWithDesc(name, cfg.Generators[name].Description))
		}
	}
	return names
}

// completeNew completes adder new, which parses its own flags: the generator, then its flags, then the values
// of an input with values or valuesFrom.
func completeNew(opts *globalOptions) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		pre := pflag.NewFlagSet("new", pflag.ContinueOnError)
		pre.ParseErrorsAllowlist.UnknownFlags = true
		pre.SetOutput(io.Discard)
		pre.StringVar(&opts.root, "root", opts.root, "")
		pre.StringVarP(&opts.config, "config", "c", opts.config, "")
		_ = pre.Parse(args)
		cfg, root, err := opts.load()
		if err != nil {
			return nil, noFiles
		}

		name := generatorArg(args)
		gen, ok := cfg.Generators[name]
		switch {
		case name == "":
			if strings.HasPrefix(toComplete, "-") {
				return nil, noFiles // cobra completes the global flags
			}
			return generatorNames(cfg, nil, toComplete), noFiles
		case !ok:
			return nil, noFiles
		}

		flag, value, hasValue := strings.Cut(strings.TrimPrefix(toComplete, "--"), "=")
		if !hasValue && len(args) > 0 && strings.HasPrefix(args[len(args)-1], "--") && !strings.Contains(args[len(args)-1], "=") {
			flag, value, hasValue = strings.TrimPrefix(args[len(args)-1], "--"), toComplete, true
			if flag == "inputs" {
				return []cobra.Completion{"json"}, cobra.ShellCompDirectiveFilterFileExt
			}
		}
		if hasValue {
			prefix := ""
			if strings.Contains(toComplete, "=") {
				prefix = "--" + flag + "="
			}
			return inputValues(cmd, cfg, root, name, gen, flag, value, prefix), noFiles
		}
		if strings.HasPrefix(toComplete, "-") {
			return generatorFlags(gen, toComplete), noFiles
		}
		return nil, noFiles
	}
}

// generatorArg returns the generator named in the words of adder new, skipping flags and their values.
func generatorArg(args []string) string {
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--root" || arg == "--config" || arg == "-c":
			i++
		case !strings.HasPrefix(arg, "-"):
			return arg
		}
	}
	return ""
}

func generatorFlags(gen config.Generator, prefix string) []cobra.Completion {
	flags := []cobra.Completion{
		cobra.CompletionWithDesc("--dry-run", "show what would change without writing anything"),
		cobra.CompletionWithDesc("--diff", "also show each change as a unified diff"),
		cobra.CompletionWithDesc("--inputs", "read inputs from a JSON file, or - for standard input"),
	}
	for _, input := range slices.Sorted(maps.Keys(gen.Inputs)) {
		flags = append(flags, cobra.CompletionWithDesc("--"+config.Flag(input), gen.Inputs[input].Help))
	}
	return slices.DeleteFunc(flags, func(c cobra.Completion) bool { return !strings.HasPrefix(c, prefix) })
}

// inputValues completes the values the input behind flag allows, each written after prefix.
func inputValues(cmd *cobra.Command, cfg *config.Config, root, name string, gen config.Generator, flag, value, prefix string) []cobra.Completion {
	input := ""
	for key, in := range gen.Inputs {
		if config.Flag(key) == flag && in.Type == config.InputString {
			input = key
		}
	}
	if input == "" || (gen.Inputs[input].Values == nil && gen.Inputs[input].ValuesFrom == "") {
		return nil
	}
	src, closeSources, err := derive.Open(cfg, root)
	if err != nil {
		return nil
	}
	defer closeSources()
	d, err := scaffold.Describe(cmd.Context(), cfg, src, name)
	if err != nil {
		return nil
	}
	var values []cobra.Completion
	for _, in := range d.Inputs {
		if in.Name != input {
			continue
		}
		for _, v := range in.Values {
			if strings.HasPrefix(v, value) {
				values = append(values, prefix+v)
			}
		}
	}
	return values
}

// completeExplain completes the files adder generates that exist, as paths from the current directory. Paths
// use forward slashes everywhere, which every shell accepts, and what was typed matches with either separator.
func completeExplain(opts *globalOptions) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, noFiles
		}
		cfg, root, err := opts.load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		src, closeSources, err := derive.Open(cfg, root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		defer closeSources()
		p, err := derive.Plan(cmd.Context(), cfg, src)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		typed := filepath.ToSlash(toComplete)
		var files []cobra.Completion
		for _, c := range p.Changes {
			if c.Status == plan.Created {
				continue // not there yet
			}
			rel, err := filepath.Rel(cwd, filepath.Join(absRoot, filepath.FromSlash(c.Path)))
			if rel = filepath.ToSlash(rel); err == nil && strings.HasPrefix(rel, typed) {
				files = append(files, rel)
			}
		}
		return files, noFiles
	}
}

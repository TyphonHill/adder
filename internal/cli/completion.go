package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/plan"
	"typhonhill.dev/adder/internal/scaffold"
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

// completeNew completes adder new, which parses its own flags: the generator, then its inputs as name=, then
// the values of an input with values or valuesFrom, and the generator command's flags.
func completeNew(opts *globalOptions) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		preParse(opts, args)
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

		switch input, value, hasValue := strings.Cut(toComplete, "="); {
		case len(args) > 0 && args[len(args)-1] == "--inputs":
			return []cobra.Completion{"json"}, cobra.ShellCompDirectiveFilterFileExt
		case strings.HasPrefix(toComplete, "-"):
			return generatorFlags(generatorCommand(opts, cfg, root, name), toComplete), noFiles
		case hasValue:
			return inputValues(cmd, opts, cfg, root, name, gen, input, value), noFiles
		default:
			return inputNames(gen, args, toComplete), noFiles | cobra.ShellCompDirectiveNoSpace
		}
	}
}

// inputNames completes the inputs of gen that args does not give yet, as name= for a string input and the bare
// name for a bool one.
func inputNames(gen config.Generator, args []string, prefix string) []cobra.Completion {
	given := make(map[string]bool)
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		given[name] = true
	}
	var names []cobra.Completion
	for _, name := range slices.Sorted(maps.Keys(gen.Inputs)) {
		in := gen.Inputs[name]
		arg := name + "="
		if in.Type == config.InputBool {
			arg = name
		}
		if !given[name] && strings.HasPrefix(arg, prefix) {
			names = append(names, cobra.CompletionWithDesc(arg, in.Help))
		}
	}
	return names
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

// generatorFlags completes the flags of a generator's command, leaving adder's global flags to cobra.
func generatorFlags(cmd *cobra.Command, prefix string) []cobra.Completion {
	var flags []cobra.Completion
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if name := "--" + f.Name; strings.HasPrefix(name, prefix) {
			flags = append(flags, cobra.CompletionWithDesc(name, f.Usage))
		}
	})
	return flags
}

// inputValues completes the values the string input called input allows, as input=value.
func inputValues(cmd *cobra.Command, opts *globalOptions, cfg *config.Config, root, name string, gen config.Generator, input, value string) []cobra.Completion {
	in, ok := gen.Inputs[input]
	if !ok || in.Type != config.InputString || (in.Values == nil && in.ValuesFrom == "") {
		return nil
	}
	src, closeSources, err := opts.open(cfg, root)
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
				values = append(values, input+"="+v)
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
		src, closeSources, err := opts.open(cfg, root)
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

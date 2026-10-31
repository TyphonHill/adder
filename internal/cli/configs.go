package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/scan"
)

// configRun is one configuration a command runs over.
type configRun struct {
	cfg  *config.Config
	root string
	// prefix is the root as a slash-separated path from the current directory, written before the paths a command
	// prints when it runs over several configurations. It is empty for one, and for the current directory.
	prefix string
	// several reports whether the command runs over several configurations.
	several bool
}

// multiReport is what a command reports with --json when it runs over several configurations: one report for
// each, with its own problems, and the problems that concern no one configuration.
type multiReport struct {
	Configs []json.RawMessage `json:"configs"`
	problems
}

// isPattern reports whether a command argument names directories, such as ./... or services/api, rather than a
// generator.
func isPattern(arg string) bool {
	return arg == "." || arg == ".." || arg == "..." || strings.ContainsAny(arg, `/\`)
}

// patternArgs checks that every argument is a directory pattern.
func patternArgs(cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		if !isPattern(arg) {
			return &usageError{fmt.Errorf("%q is not a directory; name directories such as ./... or ./services/api", arg), cmd.CommandPath()}
		}
	}
	return nil
}

// runConfigs runs fn over the nearest configuration, or, with directory patterns, over every configuration they
// match, one after another. A problem in one does not stop the others. newReport returns an empty report for
// one run.
func runConfigs(cmd *cobra.Command, opts *globalOptions, patterns []string, newReport func() report, fn func(configRun, report) error) error {
	if len(patterns) == 0 {
		rep := newReport()
		opts.report = rep
		cfg, root, err := opts.load()
		if err != nil {
			return err
		}
		return fn(configRun{cfg: cfg, root: root}, rep)
	}

	multi := &multiReport{Configs: []json.RawMessage{}}
	opts.report = multi
	if opts.root != "" || opts.config != "" {
		return &usageError{errors.New("--root and --config cannot be combined with directory patterns"), cmd.CommandPath()}
	}
	files, err := findConfigs(patterns)
	if err != nil {
		return err
	}
	outcome := &configsError{}
	base := cmd.Context()
	defer cmd.SetContext(base)
	for _, file := range files {
		rep := newReport()
		warnings := &diag.Warnings{}
		cmd.SetContext(diag.WithWarnings(base, warnings))
		cfg, err := config.Load(file, opts.packs)
		if err == nil {
			warnings.Add(cfg.Warnings...)
			prefix := filepath.ToSlash(filepath.Dir(file))
			if prefix == "." {
				prefix = ""
			}
			err = fn(configRun{cfg: cfg, root: filepath.Dir(file), prefix: prefix, several: true}, rep)
		}
		rep.setWarnings(warnings.List())
		opts.configWarnings = append(opts.configWarnings, warnings.List()...)
		rep.setProblems(problemsFrom(err))
		encoded, marshalErr := json.Marshal(rep)
		if marshalErr != nil {
			return marshalErr
		}
		name, _ := json.Marshal(filepath.ToSlash(file))
		multi.Configs = append(multi.Configs, json.RawMessage(`{"config":`+string(name)+`,`+string(encoded[1:])))
		outcome.add(err)
	}
	return outcome.err()
}

// findConfigs returns the configuration files the patterns name, as paths from the current directory. A pattern
// ending in ... names every adder.yaml at or below its directory, skipping the directories wildcards skip.
func findConfigs(patterns []string) ([]string, error) {
	var files []string
	var problems diag.List
	for _, pattern := range patterns {
		base, recursive := strings.CutSuffix(filepath.ToSlash(pattern), "...")
		base = filepath.Clean(filepath.FromSlash(cmp.Or(strings.TrimSuffix(base, "/"), ".")))
		if !recursive {
			file := filepath.Join(base, config.FileName)
			if _, err := os.Stat(file); err != nil {
				problems = append(problems, diag.Wrap(diag.IO, diag.Pos{File: file}, err))
				continue
			}
			files = append(files, file)
			continue
		}
		found := 0
		err := filepath.WalkDir(base, func(name string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && name != base && scan.Ignored(d.Name()):
				return filepath.SkipDir
			case !d.IsDir() && d.Name() == config.FileName && d.Type().IsRegular():
				files = append(files, name)
				found++
			}
			return nil
		})
		if err != nil {
			problems = append(problems, diag.Wrap(diag.IO, diag.Pos{File: base}, err))
			continue
		}
		if found == 0 {
			problems = append(problems, &diag.Diagnostic{
				Code:    diag.NoMatch,
				Message: fmt.Sprintf("pattern %q matches no %s", pattern, config.FileName),
				Hint:    "patterns name directories, such as ./... for every configuration below the current directory",
			})
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

// configsError is the outcome of running over several configurations. Each configuration's problems are in its
// own report, so with --json there is nothing to add; for people, every problem is printed.
type configsError struct {
	problems diag.List
	usage    bool
	failed   bool
	stale    int
}

func (e *configsError) add(err error) {
	switch stale, isStale := errors.AsType[*staleError](err); {
	case err == nil:
	case isStale:
		e.stale += stale.count
	default:
		_, usage := errors.AsType[*usageError](err)
		if usage || invocationProblem(err) {
			e.usage = true
		} else {
			e.failed = true
		}
		e.problems = append(e.problems, problemsFrom(err)...)
	}
}

func (e *configsError) err() error {
	if e.usage || e.failed || e.stale > 0 {
		return e
	}
	return nil
}

func (e *configsError) Error() string {
	if e.stale > 0 {
		return strings.TrimSpace(e.problems.Error() + "\n" + (&staleError{count: e.stale}).Error())
	}
	return e.problems.Error()
}

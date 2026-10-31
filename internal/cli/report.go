package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/plan"
)

// report is the result a command prints with --json.
type report interface {
	setWarnings(diag.List)
	setProblems(diag.List)
}

// problems is embedded last in every report, so each one ends with the warnings and problems found, even when
// there are none.
type problems struct {
	// Warnings did not stop the command, such as uses of something deprecated.
	Warnings diag.List `json:"warnings"`
	Problems diag.List `json:"problems"`
}

func (p *problems) setWarnings(list diag.List) { p.Warnings = list }
func (p *problems) setProblems(list diag.List) { p.Problems = list }

// changeReport is one file in a plan.
type changeReport struct {
	Path   string      `json:"path"`
	Status plan.Status `json:"status"`
	// Diff is set with --diff for changes that write a file.
	Diff string `json:"diff,omitempty"`
}

// planReport is what check and init report.
type planReport struct {
	Changes []changeReport `json:"changes"`
	problems
}

// deriveReport is what derive reports.
type deriveReport struct {
	DryRun  bool           `json:"dryRun"`
	Changes []changeReport `json:"changes"`
	problems
}

// newReport is what a generator reports.
type newReport struct {
	Generator string         `json:"generator"`
	DryRun    bool           `json:"dryRun"`
	Changes   []changeReport `json:"changes"`
	// Message is the generator's rendered message, empty when it has none.
	Message string `json:"message"`
	problems
}

// changesReport lists every change in a plan, with diffs when withDiff is set.
func changesReport(changes []plan.Change, withDiff bool) []changeReport {
	list := make([]changeReport, 0, len(changes))
	for _, c := range changes {
		r := changeReport{Path: c.Path, Status: c.Status}
		if withDiff {
			r.Diff = string(c.Diff())
		}
		list = append(list, r)
	}
	return list
}

// writeReport prints rep with warnings and the problems in err as one JSON object. A command that printed help
// instead of running has no report, and nothing is printed.
func writeReport(w io.Writer, rep report, warnings diag.List, err error) {
	if rep == nil {
		if err == nil {
			return
		}
		rep = &problems{}
	}
	rep.setWarnings(warnings)
	rep.setProblems(problemsFrom(err))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(rep) // nothing useful can be done when standard output fails
}

// problemsFrom returns the problems in err for a report, never nil. Stale outputs are not problems: the report's
// changes show them and the exit status says so.
func problemsFrom(err error) diag.List {
	if usage, ok := errors.AsType[*usageError](err); ok {
		return diag.List{{
			Code:    diag.Usage,
			Message: usage.err.Error(),
			Hint:    "run '" + usage.command() + " --help' for usage",
		}}
	}
	if _, ok := errors.AsType[*staleError](err); ok || err == nil {
		return diag.List{}
	}
	if _, ok := errors.AsType[*configsError](err); ok {
		return diag.List{} // each configuration's problems are in its own report
	}
	if errors.Is(err, context.Canceled) {
		return diag.List{{Code: diag.Other, Message: "interrupted"}}
	}
	return diag.From(err)
}

// wantsJSON reports whether args ask for JSON output, so that mistakes found before the flag is parsed are
// reported as JSON too.
func wantsJSON(args []string) bool {
	on := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			on = true
		} else if value, ok := strings.CutPrefix(arg, "--json="); ok {
			on, _ = strconv.ParseBool(value)
		}
	}
	return on
}

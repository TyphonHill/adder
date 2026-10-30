package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/pack"
	"github.com/TyphonHill/adder/internal/plan"
	"github.com/TyphonHill/adder/internal/scan"
)

// ANSI colors for the words people scan for.
const (
	green  = "32"
	yellow = "33"
	red    = "31"
	dim    = "2"
)

var statusColor = map[plan.Status]string{
	plan.Unchanged: dim,
	plan.Created:   green,
	plan.Updated:   yellow,
	plan.Skipped:   dim,
	plan.Removed:   red,
}

// printer writes the text people read, honoring --quiet, --verbose, and color.
type printer struct {
	w       io.Writer
	quiet   bool
	verbose bool
	color   bool
	prefix  string // written before each path, when several configurations run
	packs   *pack.Registry
}

func (o *globalOptions) printer(w io.Writer) *printer {
	return &printer{w: w, quiet: o.quiet, verbose: o.verbose, color: useColor(w), packs: o.packs}
}

// useColor reports whether w should get color: a terminal, without NO_COLOR or TERM=dumb, whose color support
// is on.
func useColor(w io.Writer) bool {
	return isTerminal(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && enableColor(w)
}

func (p *printer) paint(color, s string) string {
	if !p.color || color == "" {
		return s
	}
	return "\x1b[" + color + "m" + s + "\x1b[0m"
}

// change prints one change, as "would be ..." when it is only planned. Unchanged files are printed only with
// --verbose, and nothing is printed with --quiet.
func (p *printer) change(c plan.Change, planned bool) {
	if p.quiet || (c.Status == plan.Unchanged && !p.verbose) {
		return
	}
	verb := ""
	if planned && c.Status != plan.Unchanged && c.Status != plan.Skipped {
		verb = "would be "
	}
	fmt.Fprintf(p.w, "%s%s  %s\n", verb, p.paint(statusColor[c.Status], fmt.Sprintf("%-9s", c.Status)), path.Join(p.prefix, c.Path))
}

// printf prints text unless --quiet is set.
func (p *printer) printf(format string, args ...any) {
	if !p.quiet {
		fmt.Fprintf(p.w, format, args...)
	}
}

// diffs prints each change as a unified diff, after a blank line. They were asked for, so --quiet does not hide
// them.
func (p *printer) diffs(changes []plan.Change) {
	for _, c := range changes {
		c.Path = path.Join(p.prefix, c.Path) // so the patch applies from the current directory
		fmt.Fprintf(p.w, "\n%s", c.Diff())
	}
}

// scans prints how many items each scan found, with --verbose.
func (p *printer) scans(ctx context.Context, cfg *config.Config, root string) error {
	if !p.verbose {
		return nil
	}
	src, closeSources, err := derive.Open(cfg, root, p.packs)
	if err != nil {
		return err
	}
	defer closeSources()
	for _, use := range append([]string{""}, slices.Sorted(maps.Keys(cfg.Packs))...) {
		scans := cfg.Derive.Scans
		if use != "" {
			scans = cfg.Packs[use].Config.Derive.Scans
		}
		if len(scans) == 0 {
			continue
		}
		_, data, err := derive.Prepare(ctx, cfg, src, use)
		if err != nil {
			return err
		}
		for _, name := range slices.Sorted(maps.Keys(scans)) {
			n := len(data[name].([]scan.Item))
			items := "items"
			if n == 1 {
				items = "item"
			}
			where := ""
			if use != "" {
				where = " for " + use
			}
			if p.prefix != "" {
				where += " in " + p.prefix
			}
			fmt.Fprintf(p.w, "%s  scan %s%s found %d %s\n", p.paint(dim, fmt.Sprintf("%-9s", "scanned")), name, where, n, items)
		}
	}
	return nil
}

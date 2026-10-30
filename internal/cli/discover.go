package cli

import (
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/plan"
	"typhonhill.dev/adder/internal/scaffold"
)

// listReport is what list reports.
type listReport struct {
	Generators []generatorReport `json:"generators"`
	Outputs    []config.Output   `json:"outputs"`
	problems
}

type generatorReport struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Pos         diag.Pos `json:"pos,omitzero"`
	// Pack and Use name the pack and the use a generator comes from.
	Pack string `json:"pack,omitempty"`
	Use  string `json:"use,omitempty"`
}

// describeReport is what describe reports.
type describeReport struct {
	*scaffold.Description
	problems
}

// explainReport is what explain reports.
type explainReport struct {
	*derive.Explanation
	problems
}

func newListCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list [directories]",
		Short: "List the generators and derived outputs in adder.yaml",
		Args:  patternArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			newReport := func() report { return &listReport{Generators: []generatorReport{}, Outputs: []config.Output{}} }
			return runConfigs(cmd, opts, args, newReport, func(run configRun, r report) error {
				rep := r.(*listReport)
				for _, name := range slices.Sorted(maps.Keys(run.cfg.Generators)) {
					gen := run.cfg.Generators[name]
					rep.Generators = append(rep.Generators, generatorReport{Name: name, Description: gen.Description, Pos: gen.Pos, Pack: gen.Pack, Use: gen.Use})
				}
				rep.Outputs = append(rep.Outputs, run.cfg.Outputs()...)
				if !opts.json && !opts.quiet {
					if run.several {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\n", path.Join(run.prefix, config.FileName))
					}
					printList(cmd.OutOrStdout(), rep)
				}
				return nil
			})
		},
	}
}

func printList(w io.Writer, rep *listReport) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "generators")
	if len(rep.Generators) == 0 {
		fmt.Fprintln(tw, "  none")
	}
	for _, gen := range rep.Generators {
		from := ""
		if gen.Use != "" {
			from = fmt.Sprintf(" (pack %s for %s)", gen.Pack, gen.Use)
		}
		fmt.Fprintf(tw, "  %s\t%s%s\n", gen.Name, gen.Description, from)
	}
	fmt.Fprintln(tw, "outputs")
	if len(rep.Outputs) == 0 {
		fmt.Fprintln(tw, "  none")
	}
	for _, out := range rep.Outputs {
		from := "from " + out.Source()
		if out.Each != "" {
			from += ", one per " + out.Each + " item"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", out.Path, from)
	}
	_ = tw.Flush() // nothing useful can be done when standard output fails
}

func newDescribeCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <generator>",
		Short: "Show a generator's inputs and files",
		Long: `describe shows what a generator takes and creates. Values allowed by valuesFrom and rejected by unique
are computed from the scans, so they are current. With --json, it includes inputSchema, a JSON Schema
for the object adder new --inputs accepts.`,
		Args:              usageArgs(cobra.ExactArgs(1)),
		ValidArgsFunction: completeGenerators(opts, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := &describeReport{}
			opts.report = rep
			cfg, root, err := opts.load()
			if err != nil {
				return err
			}
			if _, ok := cfg.Generators[args[0]]; !ok {
				return unknownGenerator(generatorsCommand(opts, cfg, root), cfg, args[0])
			}
			src, closeSources, err := opts.open(cfg, root)
			if err != nil {
				return err
			}
			defer closeSources()
			if rep.Description, err = scaffold.Describe(cmd.Context(), cfg, src, args[0]); err != nil {
				return err
			}
			if !opts.json && !opts.quiet {
				printDescription(cmd.OutOrStdout(), rep.Description)
			}
			return nil
		},
	}
}

func printDescription(w io.Writer, d *scaffold.Description) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s: %s\n\ninputs\n", d.Name, d.Description)
	if len(d.Inputs) == 0 {
		fmt.Fprintln(tw, "  none")
	}
	for _, in := range d.Inputs {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", in.Flag, in.Type, inputNotes(in))
	}
	fmt.Fprintln(tw, "files")
	for _, f := range d.Files {
		from := "from " + f.Template
		switch {
		case f.When != "":
			from += ", with --" + config.Flag(f.When)
		case f.Unless != "":
			from += ", without --" + config.Flag(f.Unless)
		}
		if f.IfExists == config.IfExistsSkip {
			from += ", skipped if it exists"
		}
		fmt.Fprintf(tw, "  %s\t%s\n", f.Path, from)
	}
	_ = tw.Flush() // nothing useful can be done when standard output fails
	if d.Derive {
		fmt.Fprintln(w, "\nDerived outputs are updated in the same run.")
	}
}

// inputNotes is an input's help followed by its restrictions, with the values its scans allow now.
func inputNotes(in scaffold.DescribedInput) string {
	var notes []string
	switch {
	case in.Required:
		notes = append(notes, "required")
	case in.Default != nil:
		notes = append(notes, fmt.Sprintf("default %q", *in.Default))
	}
	if in.Pattern != "" {
		notes = append(notes, "must match "+in.Pattern)
	}
	switch {
	case in.Values != nil && len(in.Values) == 0:
		notes = append(notes, "no value is allowed: "+in.ValuesFrom+" has none")
	case in.Values != nil:
		notes = append(notes, "one of: "+strings.Join(in.Values, ", "))
	}
	if len(in.Existing) > 0 {
		notes = append(notes, "not one of: "+strings.Join(in.Existing, ", "))
	}
	if len(notes) == 0 {
		return in.Help
	}
	return strings.TrimSpace(fmt.Sprintf("%s (%s)", in.Help, strings.Join(notes, "; ")))
}

func newExplainCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <file>",
		Short: "Show which output and template produced a file",
		Long: `explain says where a generated file comes from: the output in adder.yaml that writes it, its
template, the scans the template refers to, and whether the file is up to date.`,
		Args:              usageArgs(cobra.ExactArgs(1)),
		ValidArgsFunction: completeExplain(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := &explainReport{}
			opts.report = rep
			cfg, root, err := opts.loadFrom(filepath.Dir(args[0])) // the configuration nearest the file
			if err != nil {
				return err
			}
			name, err := rootRelative(root, args[0])
			if err != nil {
				return err
			}
			src, closeSources, err := opts.open(cfg, root)
			if err != nil {
				return err
			}
			defer closeSources()
			if rep.Explanation, err = derive.Explain(cmd.Context(), cfg, src, name); err != nil {
				return err
			}
			if !opts.json && !opts.quiet {
				printExplanation(cmd.OutOrStdout(), rep.Explanation, filepath.ToSlash(args[0]))
			}
			return nil
		},
	}
}

// rootRelative returns file, a path from the current directory, as a slash-separated path relative to root.
func rootRelative(root, file string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", diag.Wrap(diag.IO, diag.Pos{File: root}, err)
	}
	absFile, err := filepath.Abs(file)
	if err != nil {
		return "", diag.Wrap(diag.IO, diag.Pos{File: file}, err)
	}
	rel, err := filepath.Rel(absRoot, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", diag.Errorf(diag.UnsafePath, diag.Pos{File: file}, "file is outside the repository root %s", root)
	}
	return filepath.ToSlash(rel), nil
}

// printExplanation describes e, naming the file as shown, the path it was given as.
func printExplanation(w io.Writer, e *derive.Explanation, shown string) {
	switch {
	case !e.Generated:
		fmt.Fprintf(w, "%s was not generated by adder.\n", shown)
		return
	case e.Output == nil:
		fmt.Fprintf(w, "%s was generated by adder from %s, but no output in adder.yaml writes it.\n", shown, e.Source)
		fmt.Fprintln(w, "It may be left over from an output that was removed, and can be deleted.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s is generated by adder.\n", shown)
	fmt.Fprintf(tw, "  output\t%s\n", e.Output.Pos)
	switch {
	case e.Output.Use != "":
		fmt.Fprintf(tw, "  pack\t%s, used as %s\n", e.Output.Pack, e.Output.Use)
	case e.Output.Pack != "":
		fmt.Fprintf(tw, "  pack\t%s\n", e.Output.Pack)
	}
	fmt.Fprintf(tw, "  template\t%s\n", e.Output.Template)
	if e.Output.Each != "" {
		fmt.Fprintf(tw, "  each\t%s\n", e.Output.Each)
	}
	scans := strings.Join(e.Scans, ", ")
	if scans == "" {
		scans = "none"
	}
	fmt.Fprintf(tw, "  scans\t%s\n", scans)
	fmt.Fprintf(tw, "  status\t%s\n", statusText(*e.Status))
	_ = tw.Flush() // nothing useful can be done when standard output fails
}

// statusText describes what derive would do to a file that exists: leave it, update it, or remove it.
func statusText(s plan.Status) string {
	switch s {
	case plan.Unchanged:
		return "up to date"
	case plan.Updated:
		return "out of date; run adder derive to update it"
	default:
		return "stale: no item produces it any more, and adder derive removes it"
	}
}

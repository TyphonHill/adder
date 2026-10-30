package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/scaffold"
)

// isTerminal reports whether v is a terminal. It is a variable so tests can pretend.
var isTerminal = func(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// promptMissing asks for the required inputs of the generator called name that inputs lacks, and adds them.
func promptMissing(cmd *cobra.Command, cfg *config.Config, root, name string, inputs map[string]any) error {
	missing := false
	for input, in := range cfg.Generators[name].Inputs {
		if _, given := inputs[input]; !given && in.Required() {
			missing = true
		}
	}
	if !missing {
		return nil
	}
	src, closeSources, err := derive.Open(cfg, root)
	if err != nil {
		return err
	}
	defer closeSources()
	d, err := scaffold.Describe(cmd.Context(), cfg, src, name)
	if err != nil {
		return err
	}
	answers := readLines(cmd.InOrStdin())
	for _, in := range d.Inputs {
		if _, given := inputs[in.Name]; given || !in.Required {
			continue
		}
		value, err := promptInput(cmd.Context(), cmd.ErrOrStderr(), answers, in, d)
		if err != nil {
			return err
		}
		inputs[in.Name] = value
	}
	return nil
}

// lines reads lines from a reader in the background, so a prompt waiting for one can be canceled.
type lines struct {
	ch  chan string
	err error // set when ch closes: io.EOF or a read error
}

func readLines(r io.Reader) *lines {
	l := &lines{ch: make(chan string)}
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			l.ch <- scanner.Text()
		}
		l.err = scanner.Err()
		if l.err == nil {
			l.err = io.EOF
		}
		close(l.ch)
	}()
	return l
}

// next returns the next line, or an error when the input ends or ctx is canceled.
func (l *lines) next(ctx context.Context) (string, error) {
	select {
	case line, ok := <-l.ch:
		if !ok {
			return "", l.err
		}
		return line, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// promptInput asks for the string input in on w until the answer is valid, and returns it. An input with
// allowed values lists them, numbered, and accepts a number or a value.
func promptInput(ctx context.Context, w io.Writer, answers *lines, in scaffold.DescribedInput, d *scaffold.Description) (string, error) {
	fmt.Fprintf(w, "%s: %s\n", in.Name, in.Help)
	if notes := promptNotes(in); notes != "" {
		fmt.Fprintf(w, "  %s\n", notes)
	}
	for i, v := range in.Values {
		fmt.Fprintf(w, "  %d) %s\n", i+1, v)
	}
	for {
		fmt.Fprint(w, "> ")
		answer, err := answers.next(ctx)
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(w)
			return "", &diag.Diagnostic{Code: diag.MissingInput, Message: in.Flag + " is required", Hint: in.Help}
		}
		if err != nil {
			return "", err
		}
		answer = strings.TrimSpace(answer)
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(in.Values) {
			answer = in.Values[n-1]
		}
		if answer == "" {
			fmt.Fprintln(w, "  a value is required")
			continue
		}
		if err := d.Check(in.Name, answer); err != nil {
			for _, p := range diag.From(err) {
				fmt.Fprintf(w, "  %s\n", p.Message)
			}
			continue
		}
		return answer, nil
	}
}

// promptNotes describes what an answer must be, apart from the listed values.
func promptNotes(in scaffold.DescribedInput) string {
	var notes []string
	if in.Pattern != "" {
		notes = append(notes, "must match "+in.Pattern)
	}
	if len(in.Existing) > 0 {
		notes = append(notes, "not one of: "+strings.Join(in.Existing, ", "))
	}
	if in.Values != nil && len(in.Values) == 0 {
		notes = append(notes, "no value is allowed: "+in.ValuesFrom+" has none")
	}
	return strings.Join(notes, "; ")
}

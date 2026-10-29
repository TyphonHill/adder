// Package render executes templates into file content.
package render

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/scanner"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/TyphonHill/adder/internal/diag"
)

// File executes the template name from fsys with data. Output for Go files is gofmt-formatted.
func File(fsys fs.FS, name, outPath string, data any) ([]byte, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: name}, err)
	}
	// Templates checked out with CRLF endings must render the same as everywhere else.
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))

	tmpl, err := template.New(name).
		Funcs(Funcs()).
		Option("missingkey=error").
		Parse(string(src))
	if err != nil {
		return nil, templateError(diag.TemplateSyntax, name, err, data)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, templateError(diag.TemplateExec, name, err, data)
	}
	if path.Ext(outPath) != ".go" {
		return buf.Bytes(), nil
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, invalidGo(name, outPath, buf.Bytes(), err)
	}
	return formatted, nil
}

var (
	templatePos  = regexp.MustCompile(`^template: .+?:(\d+)(?::(\d+))?: (.*)$`)
	executingAt  = regexp.MustCompile(`^executing ".*?" at (<.*?>): (.*)$`)
	missingKeyIn = regexp.MustCompile(`map has no entry for key "(.*)"$`)
)

// templateError reports a text/template error at its line and column in the template file.
func templateError(code diag.Code, name string, err error, data any) *diag.Diagnostic {
	d := &diag.Diagnostic{Code: code, Pos: diag.Pos{File: name}, Message: err.Error(), Err: err}
	if m := templatePos.FindStringSubmatch(err.Error()); m != nil {
		d.Pos.Line, _ = strconv.Atoi(m[1])
		d.Pos.Column, _ = strconv.Atoi(m[2])
		d.Message = m[3]
	}
	if m := executingAt.FindStringSubmatch(d.Message); m != nil {
		d.Message = m[1] + ": " + m[2]
	}
	if missingKeyIn.MatchString(d.Message) {
		if fields, ok := data.(map[string]any); ok {
			d.Hint = "available fields: " + strings.Join(slices.Sorted(maps.Keys(fields)), ", ")
		}
	}
	return d
}

// invalidGo reports rendered Go that does not parse, quoting the offending line of the output.
func invalidGo(name, outPath string, out []byte, err error) *diag.Diagnostic {
	d := &diag.Diagnostic{
		Code:    diag.InvalidOutput,
		Pos:     diag.Pos{File: name},
		Message: fmt.Sprintf("rendered %s is not valid Go: %v", outPath, err),
		Err:     err,
	}
	if list, ok := errors.AsType[scanner.ErrorList](err); ok && len(list) > 0 {
		first := list[0]
		d.Message = fmt.Sprintf("rendered %s is not valid Go: line %d: %s", outPath, first.Pos.Line, first.Msg)
		lines := strings.Split(string(out), "\n")
		if i := first.Pos.Line - 1; i >= 0 && i < len(lines) {
			d.Hint = "the rendered line is: " + strings.TrimSpace(lines[i])
		}
	}
	return d
}

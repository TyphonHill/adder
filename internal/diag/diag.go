// Package diag defines the structured problems adder reports to people and tools.
package diag

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Code identifies a kind of problem. Codes are stable once released, so tools may match on them.
type Code string

// Problem codes.
const (
	Other              Code = "error"
	IO                 Code = "io"
	YAMLSyntax         Code = "yaml-syntax"
	UnknownField       Code = "unknown-field"
	InvalidValue       Code = "invalid-value"
	MissingField       Code = "missing-field"
	UnsupportedVersion Code = "unsupported-version"
	InvalidName        Code = "invalid-name"
	ReservedName       Code = "reserved-name"
	UnsafePath         Code = "unsafe-path"
	DuplicatePath      Code = "duplicate-path"
	NoMatch            Code = "no-match"
	GoSyntax           Code = "go-syntax"
	TemplateSyntax     Code = "template-syntax"
	TemplateExec       Code = "template-exec"
	InvalidOutput      Code = "invalid-output"
)

// Pos is a location in a file. Line and Column are 1-based, and zero means unknown.
type Pos struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// String formats the position as file:line:column, leaving out unknown parts.
func (p Pos) String() string {
	parts := make([]string, 0, 3)
	if p.File != "" {
		parts = append(parts, p.File)
	}
	if p.Line > 0 {
		parts = append(parts, strconv.Itoa(p.Line))
		if p.Column > 0 {
			parts = append(parts, strconv.Itoa(p.Column))
		}
	}
	return strings.Join(parts, ":")
}

// Diagnostic is one problem, with where it is and how to fix it when that is known.
type Diagnostic struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Pos     Pos    `json:"pos,omitzero"`
	Hint    string `json:"hint,omitempty"`
	Err     error  `json:"-"`
}

// Errorf returns a diagnostic with a formatted message.
func Errorf(code Code, pos Pos, format string, args ...any) *Diagnostic {
	return &Diagnostic{Code: code, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// Wrap returns a diagnostic that reports err and unwraps to it.
func Wrap(code Code, pos Pos, err error) *Diagnostic {
	return &Diagnostic{Code: code, Pos: pos, Message: err.Error(), Err: err}
}

func (d *Diagnostic) Error() string {
	if pos := d.Pos.String(); pos != "" {
		return pos + ": " + d.Message
	}
	return d.Message
}

// Unwrap returns the underlying error, if any.
func (d *Diagnostic) Unwrap() error {
	return d.Err
}

// List is a set of diagnostics reported together.
type List []*Diagnostic

func (l List) Error() string {
	msgs := make([]string, len(l))
	for i, d := range l {
		msgs[i] = d.Error()
	}
	return strings.Join(msgs, "\n")
}

// Err returns l as an error, or nil when it is empty.
func (l List) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

// From returns the diagnostics carried by err. Errors that are not diagnostics are reported with the Other code.
func From(err error) List {
	if err == nil {
		return nil
	}
	if list, ok := errors.AsType[List](err); ok {
		return list
	}
	if d, ok := errors.AsType[*Diagnostic](err); ok {
		return List{d}
	}
	return List{{Code: Other, Message: err.Error(), Err: err}}
}

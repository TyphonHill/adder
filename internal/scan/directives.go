package scan

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	"github.com/TyphonHill/adder/internal/diag"
)

const directivePrefix = "//adder:"

var (
	directiveName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	argumentKey   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

// directives returns the arguments of each //adder: directive in a doc comment, by directive name.
// It returns nil when the comment has none.
func directives(fset *token.FileSet, doc *ast.CommentGroup) (map[string]map[string]string, error) {
	if doc == nil {
		return nil, nil
	}
	var found map[string]map[string]string
	for _, c := range doc.List {
		rest, ok := strings.CutPrefix(c.Text, directivePrefix)
		if !ok {
			continue
		}
		problem := func(format string, args ...any) error {
			p := fset.Position(c.Pos())
			return &diag.Diagnostic{
				Code:    diag.InvalidDirective,
				Pos:     diag.Pos{File: p.Filename, Line: p.Line, Column: p.Column},
				Message: fmt.Sprintf(format, args...),
				Hint:    `directives look like //adder:name key=value key="quoted value"`,
			}
		}

		name, argText, _ := strings.Cut(rest, " ")
		if !directiveName.MatchString(name) {
			return nil, problem("directive name %q must be lowercase letters, digits, and hyphens, starting with a letter", name)
		}
		if _, dup := found[name]; dup {
			return nil, problem("directive %q appears more than once on one declaration", name)
		}
		args, err := directiveArgs(argText)
		if err != nil {
			return nil, problem("directive %q: %v", name, err)
		}
		if found == nil {
			found = make(map[string]map[string]string)
		}
		found[name] = args
	}
	return found, nil
}

// directiveArgs parses space-separated key=value arguments, where a value may be a Go-quoted string.
func directiveArgs(text string) (map[string]string, error) {
	args := make(map[string]string)
	for {
		text = strings.TrimLeft(text, " \t")
		if text == "" {
			return args, nil
		}
		key, rest, ok := strings.Cut(text, "=")
		if !ok || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("argument %q must be key=value", nextWord(text))
		}
		if !argumentKey.MatchString(key) {
			return nil, fmt.Errorf("argument key %q is not a valid name", key)
		}
		if _, dup := args[key]; dup {
			return nil, fmt.Errorf("argument %q is given more than once", key)
		}

		var value string
		if strings.HasPrefix(rest, `"`) {
			quoted, err := strconv.QuotedPrefix(rest)
			if err != nil {
				return nil, fmt.Errorf("argument %q has an unterminated or invalid quoted value", key)
			}
			if value, err = strconv.Unquote(quoted); err != nil {
				return nil, fmt.Errorf("argument %q has an invalid quoted value", key)
			}
			rest = rest[len(quoted):]
			if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
				return nil, fmt.Errorf("argument %q must be followed by a space", key)
			}
		} else {
			value = nextWord(rest)
			rest = rest[len(value):]
		}
		args[key] = value
		text = rest
	}
}

// nextWord returns text up to the first space or tab.
func nextWord(text string) string {
	if i := strings.IndexAny(text, " \t"); i >= 0 {
		return text[:i]
	}
	return text
}

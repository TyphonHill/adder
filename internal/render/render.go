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
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/imports"
)

// Set renders templates from a file system, each with the same partials and functions loaded.
type Set struct {
	fsys     fs.FS
	partials *template.Template
	funcs    template.FuncMap
}

// NewSet parses the named partial templates from fsys. Their {{ define }} blocks are available to every
// template the set renders, and two partials may not define the same name. Extra functions are added to the
// built-in ones, and must pass [CheckFunc].
func NewSet(fsys fs.FS, partials []string, extra template.FuncMap) (*Set, error) {
	funcs := Funcs()
	maps.Copy(funcs, extra)
	base := template.New("").Funcs(funcs).Option("missingkey=error")
	definedIn := make(map[string]string)
	var problems diag.List
	for _, name := range partials {
		src, err := readTemplate(fsys, name)
		if err != nil {
			problems = append(problems, diag.From(err)...)
			continue
		}
		parsed, err := template.New(name).Funcs(funcs).Parse(src)
		if err != nil {
			problems = append(problems, templateError(diag.TemplateSyntax, name, err, nil))
			continue
		}
		for _, t := range parsed.Templates() {
			if t.Name() == name {
				continue // the text around the definitions is never rendered
			}
			if first, dup := definedIn[t.Name()]; dup {
				problems = append(problems, diag.Errorf(diag.DuplicateName, diag.Pos{File: name}, "template %q is already defined in %s", t.Name(), first))
				continue
			}
			definedIn[t.Name()] = name
			if _, err := base.AddParseTree(t.Name(), t.Tree); err != nil {
				problems = append(problems, templateError(diag.TemplateSyntax, name, err, nil))
			}
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return &Set{fsys: fsys, partials: base, funcs: funcs}, nil
}

// File executes the template name with data. Output for Go files is gofmt-formatted, without unused imports.
func (s *Set) File(name, outPath string, data any) ([]byte, error) {
	src, err := readTemplate(s.fsys, name)
	if err != nil {
		return nil, err
	}
	tmpl, err := s.partials.Clone()
	if err != nil {
		return nil, diag.Wrap(diag.TemplateSyntax, diag.Pos{File: name}, err)
	}
	if tmpl, err = tmpl.New(name).Parse(src); err != nil {
		return nil, templateError(diag.TemplateSyntax, name, err, data)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		d := templateError(diag.TemplateExec, name, err, data)
		if undefinedTemplate.MatchString(d.Message) {
			d.Hint = s.definedHint()
		}
		return nil, d
	}
	if path.Ext(outPath) != ".go" {
		return buf.Bytes(), nil
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, invalidGo(name, outPath, buf.Bytes(), err)
	}
	pruned, err := imports.Prune(formatted)
	if err != nil {
		return nil, invalidGo(name, outPath, formatted, err)
	}
	return pruned, nil
}

// Text executes a template written inline, such as a path in adder.yaml, with the set's functions. Its
// problems have no position.
func (s *Set) Text(text string, data any) (string, error) {
	const name = "inline"
	tmpl, err := template.New(name).Funcs(s.funcs).Option("missingkey=error").Parse(text)
	if err != nil {
		d := templateError(diag.TemplateSyntax, name, err, data)
		d.Pos = diag.Pos{}
		return "", d
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		d := templateError(diag.TemplateExec, name, err, data)
		d.Pos = diag.Pos{}
		return "", d
	}
	return buf.String(), nil
}

// readTemplate reads a template with its line endings normalized, so CRLF checkouts render the same everywhere.
func readTemplate(fsys fs.FS, name string) (string, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", diag.Wrap(diag.IO, diag.Pos{File: name}, err)
	}
	return strings.ReplaceAll(string(src), "\r\n", "\n"), nil
}

// definedHint lists the partials a template can use.
func (s *Set) definedHint() string {
	var names []string
	for _, t := range s.partials.Templates() {
		if t.Name() != "" {
			names = append(names, t.Name())
		}
	}
	if len(names) == 0 {
		return "no partials are defined; list templates with {{ define }} blocks under partials in adder.yaml"
	}
	slices.Sort(names)
	return "defined templates: " + strings.Join(names, ", ")
}

var (
	templatePos       = regexp.MustCompile(`^template: (.+?):(\d+)(?::(\d+))?: (.*)$`)
	executingAt       = regexp.MustCompile(`^executing ".*?" at (<.*?>): (.*)$`)
	missingKeyIn      = regexp.MustCompile(`map has no entry for key "(.*)"$`)
	undefinedTemplate = regexp.MustCompile(`template ".*" not defined$`)
)

// templateError reports a text/template error at its line and column in the template file, which may be a
// partial rather than the template named name.
func templateError(code diag.Code, name string, err error, data any) *diag.Diagnostic {
	d := &diag.Diagnostic{Code: code, Pos: diag.Pos{File: name}, Message: err.Error(), Err: err}
	if m := templatePos.FindStringSubmatch(err.Error()); m != nil {
		d.Pos.File = m[1]
		d.Pos.Line, _ = strconv.Atoi(m[2])
		if m[3] != "" {
			column, _ := strconv.Atoi(m[3])
			d.Pos.Column = column + 1 // text/template counts columns from 0
		}
		d.Message = m[4]
	}
	if m := executingAt.FindStringSubmatch(d.Message); m != nil {
		d.Message = m[1] + ": " + m[2]
		if missingKeyIn.MatchString(m[2]) {
			switch keys, ok := mapKeys(data, m[1]); {
			case ok && len(keys) == 0:
				d.Hint = "no fields are available here"
			case ok:
				d.Hint = "available fields: " + strings.Join(keys, ", ")
			}
		}
	}
	return d
}

// fieldChain matches a field reference such as <.Vars.License> or <$.Vars.License>.
var fieldChain = regexp.MustCompile(`^<\$?\.([A-Za-z0-9_.]+)>$`)

// mapKeys returns the keys of the map that a missing-key reference such as <.Vars.Licence> looked in, by following
// the reference from data. It reports false when the reference does not lead to a map with string keys.
func mapKeys(data any, ref string) ([]string, bool) {
	m := fieldChain.FindStringSubmatch(ref)
	if m == nil {
		return nil, false
	}
	fields := strings.Split(m[1], ".")
	v := reflect.ValueOf(data)
	for i, field := range fields {
		if v.Kind() == reflect.Interface {
			v = v.Elem()
		}
		if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		if i < len(fields)-1 {
			v = v.MapIndex(reflect.ValueOf(field).Convert(v.Type().Key()))
		}
	}
	keys := make([]string, 0, v.Len())
	for _, k := range v.MapKeys() {
		keys = append(keys, k.String())
	}
	slices.Sort(keys)
	return keys, true
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

// PathPattern returns a pattern matching every path the path template text can render, with each action replaced
// by a wildcard that may span directories, and the deepest directory that holds all of them. Function names are
// checked when the path renders, with the functions of the packs adder was built with.
func PathPattern(text string) (*regexp.Regexp, string, error) {
	tree := parse.New("inline")
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(text, "", "", map[string]*parse.Tree{}); err != nil {
		d := templateError(diag.TemplateSyntax, "inline", err, nil)
		d.Pos = diag.Pos{}
		return nil, "", d
	}
	var pattern, prefix strings.Builder
	pattern.WriteString("^")
	literal := true
	for _, node := range tree.Root.Nodes {
		if text, ok := node.(*parse.TextNode); ok {
			pattern.WriteString(regexp.QuoteMeta(string(text.Text)))
			if literal {
				prefix.Write(text.Text)
			}
			continue
		}
		pattern.WriteString(".*")
		literal = false
	}
	pattern.WriteString("$")
	dir := path.Dir(prefix.String())
	if !literal {
		dir = path.Dir(prefix.String() + "x") // the prefix may end inside a file name, as in mocks/mock_
	}
	return regexp.MustCompile(pattern.String()), dir, nil
}

// References returns which of names the template name refers to as fields of the top-level data, such as
// .Drivers or $.Drivers, including in the partials it calls. Fields reached indirectly, such as through index,
// are not found, and a field of an item with one of the names is counted too.
func (s *Set) References(name string, names []string) ([]string, error) {
	found := make(map[string]bool)
	err := s.inspect(name, func(_ *parse.Tree, node parse.Node) {
		switch n := node.(type) {
		case *parse.FieldNode:
			found[n.Ident[0]] = true
		case *parse.VariableNode:
			if len(n.Ident) > 1 && n.Ident[0] == "$" {
				found[n.Ident[1]] = true
			}
		}
	})
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, n := range names {
		if found[n] {
			refs = append(refs, n)
		}
	}
	slices.Sort(refs)
	return refs, nil
}

// deprecatedFuncs and deprecatedFields say what to use instead of each deprecated built-in template function and
// data field, by name. They are empty until one is deprecated, as docs/compatibility.md describes, and tests add
// to them.
var (
	deprecatedFuncs  = map[string]string{}
	deprecatedFields = map[string]string{}
)

// Deprecations returns a warning for each call of a deprecated function and each use of a deprecated data field
// in the template name and the partials it calls, at its position. Extra says what to use instead of deprecated
// functions that are not built in, such as a pack's.
func (s *Set) Deprecations(name string, extra map[string]string) (diag.List, error) {
	var warnings diag.List
	warn := func(tree *parse.Tree, node parse.Node, message, instead string) {
		d := &diag.Diagnostic{Code: diag.Deprecated, Pos: nodePos(tree, node), Message: message + ": " + instead}
		warnings = append(warnings, d)
	}
	fields := func(tree *parse.Tree, node parse.Node, idents []string) {
		for _, ident := range idents {
			if instead, ok := deprecatedFields[ident]; ok {
				warn(tree, node, fmt.Sprintf("field %q is deprecated", ident), instead)
			}
		}
	}
	err := s.inspect(name, func(tree *parse.Tree, node parse.Node) {
		switch n := node.(type) {
		case *parse.IdentifierNode:
			instead, ok := deprecatedFuncs[n.Ident]
			if !ok {
				instead, ok = extra[n.Ident]
			}
			if ok {
				warn(tree, node, fmt.Sprintf("function %q is deprecated", n.Ident), instead)
			}
		case *parse.FieldNode:
			fields(tree, node, n.Ident)
		case *parse.ChainNode:
			fields(tree, node, n.Field)
		case *parse.VariableNode:
			fields(tree, node, n.Ident[1:])
		}
	})
	return warnings, err
}

// nodePos returns where node is in the file tree was parsed from.
func nodePos(tree *parse.Tree, node parse.Node) diag.Pos {
	location, _ := tree.ErrorContext(node)
	file, line, column := location, 0, 0
	if rest, col, ok := cutLast(location); ok {
		if f, l, ok := cutLast(rest); ok {
			file, line, column = f, atoi(l), atoi(col)+1 // text/template counts columns from 0
		}
	}
	return diag.Pos{File: file, Line: line, Column: column}
}

func cutLast(s string) (before, after string, ok bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// inspect parses the template name and calls fn with every node of it and of the partials it calls, once each,
// with the tree the node belongs to.
func (s *Set) inspect(name string, fn func(*parse.Tree, parse.Node)) error {
	src, err := readTemplate(s.fsys, name)
	if err != nil {
		return err
	}
	tmpl, err := s.partials.Clone()
	if err != nil {
		return diag.Wrap(diag.TemplateSyntax, diag.Pos{File: name}, err)
	}
	if tmpl, err = tmpl.New(name).Parse(src); err != nil {
		return templateError(diag.TemplateSyntax, name, err, nil)
	}

	visited := make(map[string]bool)
	var walk func(*parse.Tree, parse.Node)
	walkTemplate := func(name string) {
		if t := tmpl.Lookup(name); t != nil && t.Tree != nil && !visited[name] {
			visited[name] = true
			walk(t.Tree, t.Root)
		}
	}
	walk = func(tree *parse.Tree, node parse.Node) {
		if node == nil || reflect.ValueOf(node).IsNil() {
			return
		}
		fn(tree, node)
		switch n := node.(type) {
		case *parse.ListNode:
			for _, child := range n.Nodes {
				walk(tree, child)
			}
		case *parse.ActionNode:
			walk(tree, n.Pipe)
		case *parse.PipeNode:
			for _, cmd := range n.Cmds {
				walk(tree, cmd)
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				walk(tree, arg)
			}
		case *parse.ChainNode:
			walk(tree, n.Node)
		case *parse.IfNode:
			walkBranch(tree, walk, &n.BranchNode)
		case *parse.RangeNode:
			walkBranch(tree, walk, &n.BranchNode)
		case *parse.WithNode:
			walkBranch(tree, walk, &n.BranchNode)
		case *parse.TemplateNode:
			walk(tree, n.Pipe)
			walkTemplate(n.Name)
		}
	}
	walkTemplate(name)
	return nil
}

func walkBranch(tree *parse.Tree, walk func(*parse.Tree, parse.Node), b *parse.BranchNode) {
	walk(tree, b.Pipe)
	walk(tree, b.List)
	walk(tree, b.ElseList)
}

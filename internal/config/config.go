// Package config loads and validates adder configuration files.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/header"
)

// FileName is the configuration file adder looks for in the repository root.
const FileName = "adder.yaml"

// Version is the configuration schema version this release understands.
const Version = 1

// buildTag is the syntax of a build tag in a //go:build line.
var buildTag = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// reservedNames are template data keys that scans cannot use.
var reservedNames = []string{"Inputs", "Item", "Module", "Vars"}

// Config is a parsed adder configuration file.
type Config struct {
	Version int `yaml:"version"`
	// Vars are string values every template sees as .Vars, such as a license name.
	Vars map[string]string `yaml:"vars"`
	// Partials are templates loaded into every template, so their {{ define }} blocks can be used anywhere.
	Partials []Partial `yaml:"partials"`
	Derive   Derive    `yaml:"derive"`
	// Generators create new files, each run as adder new <name>.
	Generators map[string]Generator `yaml:"generators"`

	// Dir is the directory holding the configuration file; template paths resolve against it.
	Dir string `yaml:"-"`
}

// Derive describes files rebuilt from scans of the repository's source code.
type Derive struct {
	Scans   map[string]Scan `yaml:"scans"`
	Outputs []Output        `yaml:"outputs"`
}

// Scan selects function and method declarations from a set of directories.
type Scan struct {
	// Dirs are slash-separated glob patterns relative to the repository root. A "**" segment matches any
	// number of directories.
	Dirs []string `yaml:"dirs"`
	// Exclude lists directory patterns, in the same syntax as Dirs, to leave out.
	Exclude      []string `yaml:"exclude"`
	IncludeTests bool     `yaml:"includeTests"`
	// BuildTags are custom build tags treated as set, such as integration. Files whose build constraints no
	// platform can satisfy are skipped.
	BuildTags []string  `yaml:"buildTags"`
	Funcs     FuncMatch `yaml:"funcs"`
	// Types selects type declarations instead of functions when set.
	Types *TypeMatch `yaml:"types"`

	// Pos is where the scan is declared, for reporting problems found while scanning.
	Pos diag.Pos `yaml:"-"`
}

// Declaration kinds, as reported on scan items and selected by kind filters.
const (
	KindFunc      = "func"
	KindMethod    = "method"
	KindStruct    = "struct"
	KindInterface = "interface"
	KindType      = "type" // any other type declaration, such as `type Status int`
)

// FuncMatch filters declarations. Unset fields match every declaration.
type FuncMatch struct {
	Exported   bool    `yaml:"exported"`
	FirstParam string  `yaml:"firstParam"`
	Name       Pattern `yaml:"name"`
	// Kind limits the scan to plain functions or to methods.
	Kind string `yaml:"kind"`
	// Receiver matches methods on this type, whether the receiver is a value or a pointer.
	Receiver string `yaml:"receiver"`
	// FirstParamType matches the first parameter's type, written with full import paths. Whitespace is ignored.
	FirstParamType string `yaml:"firstParamType"`
	// Directive matches declarations whose doc comment has //adder:<Directive>.
	Directive string `yaml:"directive"`
}

// TypeMatch filters type declarations. Unset fields match every declaration.
type TypeMatch struct {
	Exported bool    `yaml:"exported"`
	Name     Pattern `yaml:"name"`
	// Kind limits the scan to structs, interfaces, or other types.
	Kind      string `yaml:"kind"`
	Directive string `yaml:"directive"`
}

// directiveName matches the names adder directives may use, which Go also recognizes as directives.
var directiveName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Output is a file rendered from a template with the results of every scan.
type Output struct {
	// Template is a slash-separated path relative to the configuration file's directory.
	Template string `yaml:"template" json:"template"`
	// Path is a slash-separated path relative to the output root. With Each, it is a template rendered for each
	// item.
	Path string `yaml:"path" json:"path"`
	// Each names a scan; the template is rendered once per item, as .Item.
	Each string `yaml:"each" json:"each,omitempty"`

	// Pos is where the output is declared, for reporting problems found while rendering.
	Pos diag.Pos `yaml:"-" json:"pos,omitzero"`
}

// Partial is a slash-separated glob pattern of template files, relative to the configuration file's directory.
type Partial struct {
	Pattern string

	// Pos is where the pattern is written, for reporting patterns that match nothing.
	Pos diag.Pos
}

// UnmarshalYAML decodes the pattern from a plain string.
func (p *Partial) UnmarshalYAML(node *yaml.Node) error {
	return node.Decode(&p.Pattern)
}

// Pattern is a regular expression compiled when the configuration is decoded.
type Pattern struct {
	*regexp.Regexp
}

// UnmarshalYAML compiles the pattern so invalid expressions fail at load time.
func (p *Pattern) UnmarshalYAML(node *yaml.Node) error {
	var expr string
	if err := node.Decode(&expr); err != nil {
		return err
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Message: fmt.Sprintf("invalid pattern %q: %v", expr, err),
			Pos:     diag.Pos{Line: node.Line, Column: node.Column},
			Err:     err,
		}
	}
	p.Regexp = re
	return nil
}

// Find returns the path of the configuration file in dir or its nearest parent directory that has one, the way
// go finds go.mod. The path is relative to dir when dir is relative.
func Find(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", diag.Wrap(diag.IO, diag.Pos{File: dir}, err)
	}
	for at := abs; ; at = filepath.Dir(at) {
		candidate := filepath.Join(at, FileName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.Mode().IsRegular():
			if filepath.IsAbs(dir) {
				return candidate, nil
			}
			rel, err := filepath.Rel(abs, candidate)
			if err != nil {
				return candidate, nil
			}
			return filepath.Join(dir, rel), nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", diag.Wrap(diag.IO, diag.Pos{File: candidate}, err)
		}
		if filepath.Dir(at) == at {
			return "", &diag.Diagnostic{
				Code:    diag.IO,
				Message: fmt.Sprintf("no %s in %s or any parent directory", FileName, dir),
				Hint:    "run adder init to create one, or pass --root or --config",
				Err:     fs.ErrNotExist,
			}
		}
	}
}

// Load reads and validates the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: path}, err)
	}
	cfg, err := Parse(path, data)
	if err != nil {
		return nil, err
	}
	cfg.Dir = filepath.Dir(path)
	return cfg, nil
}

// Parse decodes and validates configuration data, rejecting unknown fields. File names the data in diagnostics.
func Parse(file string, data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, diag.Errorf(diag.MissingField, diag.Pos{File: file}, "configuration is empty")
		}
		return nil, decodeError(file, err)
	}

	pos := indexPositions(file, data)
	cfg.normalize()
	cfg.recordPositions(pos)
	if err := cfg.validate(pos); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// normalize cleans paths so that equivalent spellings, such as ./x and x, behave the same.
func (c *Config) normalize() {
	for i := range c.Partials {
		c.Partials[i].Pattern = cleanSlash(c.Partials[i].Pattern)
	}
	for name, scan := range c.Derive.Scans {
		for i, dir := range scan.Dirs {
			scan.Dirs[i] = cleanSlash(dir)
		}
		for i, dir := range scan.Exclude {
			scan.Exclude[i] = cleanSlash(dir)
		}
		c.Derive.Scans[name] = scan
	}
	for i := range c.Derive.Outputs {
		out := &c.Derive.Outputs[i]
		out.Template = cleanSlash(out.Template)
		if out.Each == "" { // cleaning a template could remove an action, as in {{ .Item.Dir }}/..
			out.Path = cleanSlash(out.Path)
		}
	}
	c.normalizeGenerators()
}

func cleanSlash(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

func (c *Config) recordPositions(pos positions) {
	for i := range c.Partials {
		c.Partials[i].Pos = pos.at("partials." + strconv.Itoa(i))
	}
	for name, scan := range c.Derive.Scans {
		scan.Pos = pos.at("derive.scans." + name)
		c.Derive.Scans[name] = scan
	}
	for i := range c.Derive.Outputs {
		c.Derive.Outputs[i].Pos = pos.at("derive.outputs." + strconv.Itoa(i))
	}
	c.recordGeneratorPositions(pos)
}

func (c *Config) validate(pos positions) error {
	var errs diag.List
	switch c.Version {
	case Version:
	case 0:
		errs = append(errs, diag.Errorf(diag.MissingField, pos.at(""), "version is required"))
	default:
		errs = append(errs, diag.Errorf(diag.UnsupportedVersion, pos.at("version"), "unsupported version %d, want %d", c.Version, Version))
	}

	for _, name := range slices.Sorted(maps.Keys(c.Vars)) {
		if !token.IsIdentifier(name) {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidName,
				Pos:     pos.at("vars." + name),
				Message: fmt.Sprintf("var %q: name must be a valid Go identifier", name),
				Hint:    "templates read vars as .Vars.<name>, such as .Vars.License",
			})
		}
	}
	for i, p := range c.Partials {
		if p.Pattern == "" {
			errs = append(errs, diag.Errorf(diag.MissingField, p.Pos, "partial %d: pattern is empty", i))
		} else if d := checkGlob(p.Pos, fmt.Sprintf("partial %q", p.Pattern), p.Pattern); d != nil {
			errs = append(errs, d)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(c.Derive.Scans)) {
		scan := c.Derive.Scans[name]
		at := "derive.scans." + name
		switch {
		case !token.IsIdentifier(name):
			errs = append(errs, diag.Errorf(diag.InvalidName, pos.at(at), "scan %q: name must be a valid Go identifier", name))
		case slices.Contains(reservedNames, name):
			errs = append(errs, diag.Errorf(diag.ReservedName, pos.at(at), "scan %q: name is reserved", name))
		}
		if len(scan.Dirs) == 0 {
			errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "scan %q: dirs is required", name))
		}
		for _, list := range []struct {
			key      string
			patterns []string
		}{{"dirs", scan.Dirs}, {"exclude", scan.Exclude}} {
			for i, dir := range list.patterns {
				field := fmt.Sprintf("scan %q: %s pattern %q", name, list.key, dir)
				if d := checkPattern(pos.at(at+"."+list.key+"."+strconv.Itoa(i)), field, dir); d != nil {
					errs = append(errs, d)
				}
			}
		}
		for i, tag := range scan.BuildTags {
			if !buildTag.MatchString(tag) {
				errs = append(errs, &diag.Diagnostic{
					Code:    diag.InvalidValue,
					Pos:     pos.at(at + ".buildTags." + strconv.Itoa(i)),
					Message: fmt.Sprintf("scan %q: build tag %q must be letters, digits, underscores, and dots", name, tag),
					Hint:    "list tags as //go:build lines name them, such as integration",
				})
			}
		}
		if scan.Types != nil {
			if pos.has(at + ".funcs") {
				errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".types"), "scan %q: use funcs or types, not both", name))
			}
			errs = append(errs, validateTypes(name, *scan.Types, at+".types", pos)...)
		} else {
			errs = append(errs, validateFuncs(name, scan.Funcs, at+".funcs", pos)...)
		}
	}

	seen := make(map[string]bool, len(c.Derive.Outputs))
	templates := make(map[string]int, len(c.Derive.Outputs))
	for _, out := range c.Derive.Outputs {
		templates[out.Template]++
	}
	for i, out := range c.Derive.Outputs {
		at := "derive.outputs." + strconv.Itoa(i)
		if out.Template == "" {
			errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "output %d: template is required", i))
		} else if d := checkPath(pos.at(at+".template"), fmt.Sprintf("output %d: template %q", i, out.Template), out.Template); d != nil {
			errs = append(errs, d)
		}
		if out.Each != "" {
			errs = append(errs, c.validateEach(i, out, templates[out.Template] > 1, at, pos)...)
			continue
		}
		switch {
		case strings.Contains(out.Path, "{{"):
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at + ".path"),
				Message: fmt.Sprintf("output %d: path %q is a template, but the output has no each", i, out.Path),
				Hint:    "only outputs with each, which render one file per scan item, have templated paths",
			})
		case out.Path == "":
			errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "output %d: path is required", i))
		case seen[out.Path]:
			errs = append(errs, diag.Errorf(diag.DuplicatePath, pos.at(at+".path"), "output %d: path %q is already used by another output", i, out.Path))
		default:
			if d := checkPath(pos.at(at+".path"), fmt.Sprintf("output %d: path %q", i, out.Path), out.Path); d != nil {
				errs = append(errs, d)
			}
		}
		seen[out.Path] = true
	}

	errs = append(errs, c.validateGenerators(pos)...)
	return errs.Err()
}

// validateEach checks an output that renders one file per item of a scan. Shared reports whether another output
// uses the same template.
func (c *Config) validateEach(i int, out Output, shared bool, at string, pos positions) diag.List {
	var errs diag.List
	if !c.hasScan(out.Each) {
		errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".each"), "output %d: each refers to %q, which is not a scan", i, out.Each))
	}
	if shared {
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".template"),
			Message: fmt.Sprintf("output %d: template %q is also used by another output", i, out.Template),
			Hint:    "an output with each needs a template of its own, because its stale files are found by the header naming it",
		})
	}
	if strings.TrimSpace(out.Path) == "" {
		return append(errs, diag.Errorf(diag.MissingField, pos.at(at), "output %d: path is required", i))
	}
	tail := out.Path
	if end := strings.LastIndex(tail, "}}"); end >= 0 {
		tail = tail[end+len("}}"):]
	}
	if !header.Supported(tail) {
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".path"),
			Message: fmt.Sprintf("output %d: path %q must end in a file type adder can mark as generated", i, out.Path),
			Hint:    "stale files are found by adder's header, so the path needs a literal extension with comments, such as .go, .ts, .yaml, or .sql",
		})
	}
	return errs
}

// validateFuncs checks the function filters of the scan called name.
func validateFuncs(name string, m FuncMatch, at string, pos positions) diag.List {
	var errs diag.List
	switch m.Kind {
	case "", KindFunc, KindMethod:
	default:
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".kind"),
			Message: fmt.Sprintf("scan %q: unknown kind %q", name, m.Kind),
			Hint:    "kind is func or method",
		})
	}
	errs = append(errs, validateDirective(name, m.Directive, at, pos)...)
	if m.Receiver != "" {
		if !token.IsIdentifier(m.Receiver) {
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".receiver"), "scan %q: receiver %q must be a type name", name, m.Receiver))
		}
		if m.Kind == KindFunc {
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".receiver"), "scan %q: receiver only matches methods, but kind is func", name))
		}
	}
	return errs
}

// validateTypes checks the type filters of the scan called name.
func validateTypes(name string, m TypeMatch, at string, pos positions) diag.List {
	var errs diag.List
	switch m.Kind {
	case "", KindStruct, KindInterface, KindType:
	default:
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".kind"),
			Message: fmt.Sprintf("scan %q: unknown type kind %q", name, m.Kind),
			Hint:    "kind is struct, interface, or type",
		})
	}
	return append(errs, validateDirective(name, m.Directive, at, pos)...)
}

func validateDirective(name, directive, at string, pos positions) diag.List {
	if directive == "" || directiveName.MatchString(directive) {
		return nil
	}
	return diag.List{{
		Code:    diag.InvalidValue,
		Pos:     pos.at(at + ".directive"),
		Message: fmt.Sprintf("scan %q: directive %q is not a valid directive name", name, directive),
		Hint:    "directive names are lowercase letters, digits, and hyphens, starting with a letter",
	}}
}

// checkPattern reports directory patterns that are unsafe or malformed. "**" may only appear as a whole segment.
func checkPattern(pos diag.Pos, field, pattern string) *diag.Diagnostic {
	if d := checkPath(pos, field, pattern); d != nil {
		return d
	}
	for segment := range strings.SplitSeq(pattern, "/") {
		if segment == "**" {
			continue
		}
		if strings.Contains(segment, "**") {
			return &diag.Diagnostic{Code: diag.InvalidValue, Pos: pos, Message: field + `: "**" must be a whole path segment`, Hint: `write a/**/b, not a/**b`}
		}
		if _, err := path.Match(segment, ""); err != nil {
			return diag.Errorf(diag.InvalidValue, pos, "%s: %v", field, err)
		}
	}
	return nil
}

// checkGlob reports file patterns that are unsafe or malformed. Unlike directory patterns, they have no "**".
func checkGlob(pos diag.Pos, field, pattern string) *diag.Diagnostic {
	if d := checkPath(pos, field, pattern); d != nil {
		return d
	}
	if strings.Contains(pattern, "**") {
		return &diag.Diagnostic{Code: diag.InvalidValue, Pos: pos, Message: field + `: "**" is not supported in file patterns`, Hint: "list each directory, such as templates/partials/*.tmpl"}
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return diag.Errorf(diag.InvalidValue, pos, "%s: %v", field, err)
	}
	return nil
}

// CheckPath reports a path that is not relative, slash-separated, and confined to its root, describing it as field.
func CheckPath(pos diag.Pos, field, p string) *diag.Diagnostic {
	return checkPath(pos, field, p)
}

// checkPath reports paths that are not relative, slash-separated, and confined to their root.
func checkPath(pos diag.Pos, field, p string) *diag.Diagnostic {
	switch {
	case strings.Contains(p, `\`):
		return &diag.Diagnostic{Code: diag.UnsafePath, Pos: pos, Message: field + " must use forward slashes", Hint: "paths in adder.yaml are slash-separated on every operating system"}
	case !filepath.IsLocal(filepath.FromSlash(p)):
		return diag.Errorf(diag.UnsafePath, pos, "%s must be relative and stay inside its root", field)
	}
	return nil
}

var (
	yamlLine     = regexp.MustCompile(`^(?:yaml: )?line (\d+): (.*)$`)
	unknownField = regexp.MustCompile(`^field (\S+) not found in type (\S+)$`)
)

// decodeError converts a YAML decoding error into diagnostics with line numbers.
func decodeError(file string, err error) error {
	if d, ok := errors.AsType[*diag.Diagnostic](err); ok {
		d.Pos.File = file
		return d
	}
	if typeErr, ok := errors.AsType[*yaml.TypeError](err); ok {
		var list diag.List
		for _, msg := range typeErr.Errors {
			list = append(list, lineDiagnostic(file, diag.InvalidValue, msg))
		}
		return list
	}
	return lineDiagnostic(file, diag.YAMLSyntax, err.Error())
}

func lineDiagnostic(file string, code diag.Code, msg string) *diag.Diagnostic {
	d := &diag.Diagnostic{Code: code, Pos: diag.Pos{File: file}, Message: strings.TrimPrefix(msg, "yaml: ")}
	if m := yamlLine.FindStringSubmatch(msg); m != nil {
		d.Pos.Line, _ = strconv.Atoi(m[1])
		d.Message = m[2]
	}
	if m := unknownField.FindStringSubmatch(d.Message); m != nil {
		d.Code = diag.UnknownField
		d.Message = fmt.Sprintf("unknown field %q", m[1])
		if known := knownFields[m[2]]; len(known) > 0 {
			d.Hint = "known fields: " + strings.Join(known, ", ")
		}
	}
	return d
}

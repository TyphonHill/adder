package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"typhonhill.dev/adder/internal/diag"
)

// Generator creates new files from templates. It runs as adder new <name>.
type Generator struct {
	Description string           `yaml:"description"`
	Inputs      map[string]Input `yaml:"inputs"`
	Files       []File           `yaml:"files"`
	// Message is a template printed after the files are created, such as the next steps to take.
	Message string `yaml:"message"`
	// Derive recomputes the derived outputs with the new files in place. It defaults to true.
	Derive *bool `yaml:"derive"`
	// Tests are sample inputs and the files the generator should create from them, for adder test.
	Tests []GeneratorTest `yaml:"tests"`
	// Deprecated marks the generator as on its way out, saying what to use instead. adder new warns when it runs.
	Deprecated string `yaml:"deprecated"`

	// Pos is where the generator is declared, for reporting problems found while generating.
	Pos diag.Pos `yaml:"-"`
	// Pack and Use name the pack and the use a generator comes from, for a generator a pack declares. Its
	// templates are in the pack.
	Pack, Use string `yaml:"-"`
}

// GeneratorTest is a generator run with sample inputs, compared with the files under Want.
type GeneratorTest struct {
	Name   string         `yaml:"name"`
	Inputs map[string]any `yaml:"inputs"`
	// Want is a slash-separated directory relative to the configuration file's directory, holding the files the
	// generator should create at their paths in the repository.
	Want string `yaml:"want"`

	// Pos is where the test is declared, for reporting its problems and failures.
	Pos diag.Pos `yaml:"-"`
}

// Input types.
const (
	InputString = "string"
	InputBool   = "bool"
)

// Input is a value given to a generator, on the command line as name=value or in a JSON object of inputs.
type Input struct {
	Type string `yaml:"type"`
	Help string `yaml:"help"`
	// Default makes a string input optional. Without one, the input is required.
	Default *string `yaml:"default"`
	// Pattern must match the whole value.
	Pattern Pattern `yaml:"pattern"`
	// Values lists the allowed values.
	Values []string `yaml:"values"`
	// ValuesFrom takes the allowed values from a scan, written as Scan.Field.
	ValuesFrom string `yaml:"valuesFrom"`
	// Unique rejects values a scan already has, written as Scan.Field.
	Unique string `yaml:"unique"`
	// Deprecated marks the input as on its way out, saying what to use instead. adder new warns when it is given.
	// A deprecated string input needs a default, so that leaving it out works.
	Deprecated string `yaml:"deprecated"`

	// Pos is where the input is declared, for reporting problems with its settings.
	Pos diag.Pos `yaml:"-"`

	whole *regexp.Regexp
}

// Required reports whether the input must be given.
func (in Input) Required() bool {
	return in.Type == InputString && in.Default == nil
}

// Anchored returns the input's pattern anchored to match whole values, as adder applies it, or "" without one.
func (in Input) Anchored() string {
	if in.whole == nil {
		return ""
	}
	return in.whole.String()
}

// MatchesPattern reports whether value satisfies the input's pattern, which must match all of it.
func (in Input) MatchesPattern(value string) bool {
	return in.whole == nil || in.whole.MatchString(value)
}

// File policies for a generated file that already exists.
const (
	IfExistsFail = "fail"
	IfExistsSkip = "skip"
)

// File is one file a generator creates.
type File struct {
	// Template is a slash-separated path relative to the configuration file's directory, or to the root of Pack.
	Template string `yaml:"template"`
	// Pack names the pack holding the template, for a file rendering a pack's template.
	Pack string `yaml:"pack"`
	// Path is a template for the slash-separated path of the new file, relative to the repository root.
	Path string `yaml:"path"`
	// When and Unless name a bool input that includes or excludes the file.
	When   string `yaml:"when"`
	Unless string `yaml:"unless"`
	// IfExists says what to do when the file already exists: fail or skip. Existing files are never overwritten.
	IfExists string `yaml:"ifExists"`

	// Pos is where the file is declared, for reporting problems found while generating it.
	Pos diag.Pos `yaml:"-"`
}

var (
	generatorName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	inputName     = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	scanField     = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)
)

func (c *Config) normalizeGenerators() {
	for name, gen := range c.Generators {
		for key, in := range gen.Inputs {
			if in.Type == "" {
				in.Type = InputString
			}
			if in.Pattern.Regexp != nil {
				in.whole = regexp.MustCompile(`^(?:` + in.Pattern.String() + `)$`)
			}
			gen.Inputs[key] = in
		}
		for i := range gen.Tests {
			gen.Tests[i].Want = cleanSlash(gen.Tests[i].Want)
		}
		for i := range gen.Files {
			f := &gen.Files[i]
			f.Template = cleanSlash(f.Template)
			if f.IfExists == "" {
				f.IfExists = IfExistsFail
			}
		}
		c.Generators[name] = gen
	}
}

func (c *Config) recordGeneratorPositions(pos positions) {
	for name, gen := range c.Generators {
		at := "generators." + name
		gen.Pos = pos.at(at)
		for key, in := range gen.Inputs {
			in.Pos = pos.at(at + ".inputs." + key)
			gen.Inputs[key] = in
		}
		for i := range gen.Files {
			gen.Files[i].Pos = pos.at(at + ".files." + strconv.Itoa(i))
		}
		for i := range gen.Tests {
			gen.Tests[i].Pos = pos.at(at + ".tests." + strconv.Itoa(i))
		}
		c.Generators[name] = gen
	}
}

func (c *Config) validateGenerators(pos positions) diag.List {
	var errs diag.List
	for _, name := range slices.Sorted(maps.Keys(c.Generators)) {
		gen := c.Generators[name]
		at := "generators." + name
		if !generatorName.MatchString(name) {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidName,
				Pos:     pos.at(at),
				Message: fmt.Sprintf("generator %q: name must be lowercase words joined by hyphens", name),
				Hint:    "generator names are commands, such as adder new http-handler",
			})
		}
		if len(gen.Files) == 0 {
			errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "generator %q: files is required", name))
		}

		for _, key := range slices.Sorted(maps.Keys(gen.Inputs)) {
			errs = append(errs, c.validateInput(name, key, gen.Inputs[key], at+".inputs."+key, pos)...)
		}

		for i, f := range gen.Files {
			errs = append(errs, validateFile(name, i, f, gen.Inputs, at+".files."+strconv.Itoa(i), pos)...)
		}
		names := make(map[string]bool, len(gen.Tests))
		for i, test := range gen.Tests {
			errs = append(errs, validateTest(name, i, test, gen.Inputs, names, at+".tests."+strconv.Itoa(i), pos)...)
		}
	}
	return errs
}

func (c *Config) validateInput(gen, name string, in Input, at string, pos positions) diag.List {
	var errs diag.List
	field := fmt.Sprintf("generator %q: input %q", gen, name)
	if !inputName.MatchString(name) {
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidName,
			Pos:     pos.at(at),
			Message: field + ": name must be lowerCamelCase",
			Hint:    "input names are template fields and arguments: packageName is .Inputs.packageName and packageName=...",
		})
	}

	switch in.Type {
	case InputString:
	case InputBool:
		for _, setting := range []struct {
			key string
			set bool
		}{
			{"default", in.Default != nil},
			{"pattern", in.Pattern.Regexp != nil},
			{"values", in.Values != nil},
			{"valuesFrom", in.ValuesFrom != ""},
			{"unique", in.Unique != ""},
		} {
			if setting.set {
				errs = append(errs, &diag.Diagnostic{
					Code:    diag.InvalidValue,
					Pos:     pos.at(at + "." + setting.key),
					Message: fmt.Sprintf("%s: %s only applies to string inputs", field, setting.key),
					Hint:    "bool inputs are false unless their flag is given",
				})
			}
		}
		return errs
	default:
		return append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".type"),
			Message: fmt.Sprintf("%s: unknown type %q", field, in.Type),
			Hint:    "type is string or bool",
		})
	}

	if in.Deprecated != "" && in.Default == nil {
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.MissingField,
			Pos:     pos.at(at + ".deprecated"),
			Message: field + ": a deprecated input needs a default",
			Hint:    "leaving a deprecated input out must keep working, so give it the default that matches what it did",
		})
	}
	if in.Values != nil && in.ValuesFrom != "" {
		errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".valuesFrom"), "%s: use values or valuesFrom, not both", field))
	}
	if in.Default != nil {
		def := *in.Default
		if !in.MatchesPattern(def) {
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".default"), "%s: default %q does not match the pattern %s", field, def, in.Pattern))
		}
		if in.Values != nil && !slices.Contains(in.Values, def) {
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".default"), "%s: default %q is not one of the values", field, def))
		}
	}
	for _, ref := range []struct{ key, value string }{{"valuesFrom", in.ValuesFrom}, {"unique", in.Unique}} {
		if ref.value == "" {
			continue
		}
		m := scanField.FindStringSubmatch(ref.value)
		switch {
		case m == nil:
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at + "." + ref.key),
				Message: fmt.Sprintf("%s: %s %q must name a scan and a field", field, ref.key, ref.value),
				Hint:    "write Scan.Field, such as Drivers.Package",
			})
		case !c.hasScan(m[1]):
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+"."+ref.key), "%s: %s refers to %q, which is not a scan", field, ref.key, m[1]))
		}
	}
	return errs
}

func (c *Config) hasScan(name string) bool {
	_, ok := c.Derive.Scans[name]
	return ok
}

func validateFile(gen string, i int, f File, inputs map[string]Input, at string, pos positions) diag.List {
	var errs diag.List
	field := fmt.Sprintf("generator %q: file %d", gen, i)
	if f.Template == "" {
		errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "%s: template is required", field))
	} else if d := checkPath(pos.at(at+".template"), fmt.Sprintf("%s: template %q", field, f.Template), f.Template); d != nil {
		errs = append(errs, d)
	}
	if strings.TrimSpace(f.Path) == "" {
		errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "%s: path is required", field))
	}

	if f.When != "" && f.Unless != "" {
		errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".unless"), "%s: use when or unless, not both", field))
	}
	for _, cond := range []struct{ key, input string }{{"when", f.When}, {"unless", f.Unless}} {
		if cond.input == "" {
			continue
		}
		if in, ok := inputs[cond.input]; !ok || in.Type != InputBool {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at + "." + cond.key),
				Message: fmt.Sprintf("%s: %s must name a bool input, not %q", field, cond.key, cond.input),
				Hint:    "when and unless take an input name, such as when: cache",
			})
		}
	}

	switch f.IfExists {
	case IfExistsFail, IfExistsSkip:
	default:
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".ifExists"),
			Message: fmt.Sprintf("%s: unknown ifExists %q", field, f.IfExists),
			Hint:    "ifExists is fail or skip; adder never overwrites a file people own",
		})
	}
	return errs
}

// validateTest checks a generator test. Names holds the test names seen so far. Input values are checked
// against the generator's restrictions when the test runs, since those can depend on the scans.
func validateTest(gen string, i int, test GeneratorTest, inputs map[string]Input, names map[string]bool, at string, pos positions) diag.List {
	var errs diag.List
	field := fmt.Sprintf("generator %q: test %d", gen, i)
	switch {
	case test.Name == "":
		errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "%s: name is required", field))
	case !generatorName.MatchString(test.Name):
		errs = append(errs, diag.Errorf(diag.InvalidName, pos.at(at+".name"), "%s: name %q must be lowercase words joined by hyphens", field, test.Name))
	case names[test.Name]:
		errs = append(errs, diag.Errorf(diag.DuplicateName, pos.at(at+".name"), "%s: another test is also called %q", field, test.Name))
	}
	names[test.Name] = true
	switch test.Want {
	case "":
		errs = append(errs, diag.Errorf(diag.MissingField, pos.at(at), "%s: want is required", field))
	case ".":
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at + ".want"),
			Message: field + ": want must be a directory of its own, not the configuration's directory",
			Hint:    "use a directory under testdata, such as testdata/generators/<generator>/<test>",
		})
	default:
		if d := checkPath(pos.at(at+".want"), fmt.Sprintf("%s: want %q", field, test.Want), test.Want); d != nil {
			errs = append(errs, d)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(test.Inputs)) {
		if _, ok := inputs[key]; !ok {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at + ".inputs." + key),
				Message: fmt.Sprintf("%s: unknown input %q", field, key),
				Hint:    "inputs are keyed by input name, not flag, such as packageName",
			})
		}
	}
	return errs
}

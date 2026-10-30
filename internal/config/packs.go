package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v3"

	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/render"
)

// PackFileName is the configuration file at the root of a pack's file system.
const PackFileName = "pack.yaml"

// Packs finds the packs adder was built with.
type Packs interface {
	// FS returns the file system holding the named pack's pack.yaml and templates.
	FS(name string) (fs.FS, bool)
	// Funcs returns the template functions the packs add to the built-in ones.
	Funcs() template.FuncMap
}

// Use is one use of a pack, with the options it gives.
type Use struct {
	// Pack is the pack's name.
	Pack string `yaml:"pack"`
	// With gives the pack's options, by name.
	With map[string]any `yaml:"with"`
	// Generators renames the pack's generators, from the pack's name to the command's.
	Generators map[string]string `yaml:"generators"`
	// Tests are the tests of the pack's generators, by command name, with golden files in the repository.
	Tests map[string][]GeneratorTest `yaml:"tests"`

	// Pos is where the use is declared.
	Pos diag.Pos `yaml:"-"`

	// FS holds the pack's templates, Options the value of each of its options, and Config its configuration with
	// those values filled in. They are set when the configuration loads.
	FS      fs.FS          `yaml:"-"`
	Options map[string]any `yaml:"-"`
	Config  *Config        `yaml:"-"`
}

// PackTemplates are a pack's templates and partials, as a configuration's own outputs and files render them.
type PackTemplates struct {
	FS       fs.FS
	Partials []Partial
}

// Option types.
const (
	OptionString = "string"
	OptionBool   = "bool"
	OptionList   = "list"
)

// Option is a setting a pack takes from each use, as .Options in its templates.
type Option struct {
	Type string `yaml:"type"`
	Help string `yaml:"help"`
	// Default makes the option optional. Bool options default to false.
	Default any `yaml:"default"`
	// Pattern must match the whole value, or each value of a list.
	Pattern Pattern `yaml:"pattern"`
	// Values lists the allowed values, or the values a list may hold.
	Values []string `yaml:"values"`

	// Pos is where the option is declared.
	Pos diag.Pos `yaml:"-"`

	whole *regexp.Regexp
}

// packFile is the content of a pack.yaml.
type packFile struct {
	Version    int                  `yaml:"version"`
	Options    map[string]Option    `yaml:"options"`
	Partials   []Partial            `yaml:"partials"`
	Derive     Derive               `yaml:"derive"`
	Generators map[string]Generator `yaml:"generators"`
}

// optionKey is the only key of a mapping that stands for an option's value, as in dirs: {option: dirs}.
const optionKey = "option"

// Source returns how adder's header names the output's template: its path, followed for a pack's template by the
// pack and the use.
func (o Output) Source() string {
	switch {
	case o.Pack == "":
		return o.Template
	case o.Use == "":
		return fmt.Sprintf("%s in pack %s", o.Template, o.Pack)
	default:
		return fmt.Sprintf("%s in pack %s for %s", o.Template, o.Pack, o.Use)
	}
}

// loadPacks loads the pack of every use, checking the options it gives.
func (c *Config) loadPacks(packs Packs, pos positions) diag.List {
	var errs diag.List
	for _, name := range slices.Sorted(maps.Keys(c.Packs)) {
		use := c.Packs[name]
		at := "packs." + name
		use.Pos = pos.at(at)
		var fsys fs.FS
		found := false
		if packs != nil {
			fsys, found = packs.FS(use.Pack)
		}
		switch {
		case !generatorName.MatchString(name):
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidName,
				Pos:     use.Pos,
				Message: fmt.Sprintf("pack use %q: name must be lowercase words joined by hyphens", name),
				Hint:    "a use is named like a generator, such as drivers or http-handlers",
			})
		case use.Pack == "":
			errs = append(errs, diag.Errorf(diag.MissingField, use.Pos, "pack use %q: pack is required", name))
		case !found:
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.UnknownPack,
				Pos:     pos.at(at + ".pack"),
				Message: fmt.Sprintf("pack use %q: adder was built without pack %q", name, use.Pack),
				Hint:    "packs are part of a repository's own build of adder, such as go tool adder, whose main package passes them to adder.Main",
			})
		default:
			cfg, values, problems := loadPack(use.Pack, fsys, name, use.With, packs.Funcs(), at, pos)
			errs = append(errs, problems...)
			use.FS, use.Options, use.Config = fsys, values, cfg
		}
		c.Packs[name] = use
	}
	errs = append(errs, c.loadPackTemplates(packs, pos)...)
	errs = append(errs, c.checkUsePaths()...)
	return append(errs, c.addUseGenerators(pos)...)
}

// loadPackTemplates finds the packs whose templates the configuration's own outputs and generator files name,
// with the partials their pack.yaml lists.
func (c *Config) loadPackTemplates(packs Packs, pos positions) diag.List {
	at := make(map[string]string) // the first place naming each pack
	for i, out := range c.Derive.Outputs {
		if out.Pack != "" && at[out.Pack] == "" {
			at[out.Pack] = "derive.outputs." + strconv.Itoa(i) + ".pack"
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.Generators)) {
		for i, f := range c.Generators[name].Files {
			if f.Pack != "" && at[f.Pack] == "" {
				at[f.Pack] = "generators." + name + ".files." + strconv.Itoa(i) + ".pack"
			}
		}
	}
	var errs diag.List
	for _, pack := range slices.Sorted(maps.Keys(at)) {
		var fsys fs.FS
		found := false
		if packs != nil {
			fsys, found = packs.FS(pack)
		}
		if !found {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.UnknownPack,
				Pos:     pos.at(at[pack]),
				Message: fmt.Sprintf("adder was built without pack %q", pack),
				Hint:    "packs are part of a repository's own build of adder, such as go tool adder, whose main package passes them to adder.Main",
			})
			continue
		}
		partials, problems := packPartials(pack, fsys)
		errs = append(errs, problems...)
		if c.PackTemplates == nil {
			c.PackTemplates = make(map[string]PackTemplates)
		}
		c.PackTemplates[pack] = PackTemplates{FS: fsys, Partials: partials}
	}
	return errs
}

// packPartials returns the partials a pack's pack.yaml lists, without loading the rest, which may need options.
func packPartials(pack string, fsys fs.FS) ([]Partial, diag.List) {
	file := diag.Pos{Pack: pack, File: PackFileName}
	data, err := fs.ReadFile(fsys, PackFileName)
	if err != nil {
		return nil, diag.List{diag.Wrap(diag.IO, file, err)}
	}
	var root yaml.Node
	var content struct {
		Partials []Partial `yaml:"partials"`
	}
	err = yaml.Unmarshal(data, &root)
	if err == nil {
		err = root.Decode(&content)
	}
	if err != nil {
		return nil, diag.InPack(decodeError(PackFileName, err), pack, file)
	}
	pos := indexNode(PackFileName, pack, &root)
	var errs diag.List
	for i := range content.Partials {
		p := &content.Partials[i]
		p.Pattern, p.Pos = cleanSlash(p.Pattern), pos.at("partials."+strconv.Itoa(i))
		if d := checkGlob(p.Pos, fmt.Sprintf("partial %q", p.Pattern), p.Pattern); d != nil {
			errs = append(errs, d)
		}
	}
	return content.Partials, errs
}

// Outputs returns the configuration's own outputs, then those of each use of a pack, in name order.
func (c *Config) Outputs() []Output {
	outputs := slices.Clone(c.Derive.Outputs)
	for _, name := range slices.Sorted(maps.Keys(c.Packs)) {
		if use := c.Packs[name]; use.Config != nil {
			outputs = append(outputs, use.Config.Derive.Outputs...)
		}
	}
	return outputs
}

// addUseGenerators adds the generators of every use to the configuration's, under their command names, with the
// tests the use gives them.
func (c *Config) addUseGenerators(pos positions) diag.List {
	var errs diag.List
	from := make(map[string]string, len(c.Generators))
	for name := range c.Generators {
		from[name] = FileName
	}
	for _, useName := range slices.Sorted(maps.Keys(c.Packs)) {
		use := c.Packs[useName]
		at := "packs." + useName
		if use.Config == nil {
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(use.Generators)) {
			renamed := use.Generators[name]
			switch _, ok := use.Config.Generators[name]; {
			case !ok:
				errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".generators."+name), "pack use %q: pack %q has no generator %q", useName, use.Pack, name))
			case !generatorName.MatchString(renamed):
				errs = append(errs, diag.Errorf(diag.InvalidName, pos.at(at+".generators."+name), "pack use %q: generator name %q must be lowercase words joined by hyphens", useName, renamed))
			}
		}
		commands := make(map[string]bool)
		for _, name := range slices.Sorted(maps.Keys(use.Config.Generators)) {
			gen := use.Config.Generators[name]
			command := cmp.Or(use.Generators[name], name)
			commands[command] = true
			if first, taken := from[command]; taken {
				errs = append(errs, &diag.Diagnostic{
					Code:    diag.InvalidName,
					Pos:     use.Pos,
					Message: fmt.Sprintf("pack use %q: generator %q is already defined in %s", useName, command, first),
					Hint:    fmt.Sprintf("rename it under the use, such as generators: {%s: %s-%s}", name, useName, name),
				})
				continue
			}
			from[command] = fmt.Sprintf("pack use %q", useName)
			gen.Pack, gen.Use = use.Pack, useName
			gen.Tests = use.Tests[command]
			names := make(map[string]bool, len(gen.Tests))
			for i := range gen.Tests {
				testAt := at + ".tests." + command + "." + strconv.Itoa(i)
				gen.Tests[i].Want = cleanSlash(gen.Tests[i].Want)
				gen.Tests[i].Pos = pos.at(testAt)
				errs = append(errs, validateTest(command, i, gen.Tests[i], gen.Inputs, names, testAt, pos)...)
			}
			if c.Generators == nil {
				c.Generators = make(map[string]Generator)
			}
			c.Generators[command] = gen
		}
		for _, command := range slices.Sorted(maps.Keys(use.Tests)) {
			if !commands[command] {
				errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".tests."+command), "pack use %q: tests name %q, which is not one of its generators", useName, command))
			}
		}
	}
	return errs
}

// checkUsePaths reports a file that the repository's outputs and a use, or two uses, would both write.
func (c *Config) checkUsePaths() diag.List {
	writer := make(map[string]string)
	for _, out := range c.Derive.Outputs {
		if out.Each == "" {
			writer[out.Path] = "an output in " + FileName
		}
	}
	var errs diag.List
	for _, name := range slices.Sorted(maps.Keys(c.Packs)) {
		use := c.Packs[name]
		if use.Config == nil {
			continue
		}
		for _, out := range use.Config.Derive.Outputs {
			if out.Each != "" {
				continue
			}
			if first, dup := writer[out.Path]; dup {
				errs = append(errs, diag.Errorf(diag.DuplicatePath, use.Pos, "pack use %q: path %q is already written by %s", name, out.Path, first))
				continue
			}
			writer[out.Path] = fmt.Sprintf("pack use %q", name)
		}
	}
	return errs
}

// loadPack reads the pack's pack.yaml and fills in the options use gives in with, declared at at in adder.yaml.
func loadPack(pack string, fsys fs.FS, use string, with map[string]any, funcs template.FuncMap, at string, adder positions) (*Config, map[string]any, diag.List) {
	file := diag.Pos{Pack: pack, File: PackFileName}
	data, err := fs.ReadFile(fsys, PackFileName)
	if err != nil {
		return nil, nil, diag.List{diag.Wrap(diag.IO, file, err)}
	}
	inPack := func(err error) diag.List { return diag.InPack(err, pack, file) }

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, inPack(decodeError(PackFileName, err))
	}
	if len(root.Content) == 0 {
		return nil, nil, diag.List{diag.Errorf(diag.MissingField, file, "%s is empty", PackFileName)}
	}

	// The file as written has unknown fields reported, ignoring the option mappings that stand for other types.
	placeholders := optionLines(&root)
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var written packFile
	if err := dec.Decode(&written); err != nil && !errors.Is(err, io.EOF) {
		var problems diag.List
		for _, d := range inPack(decodeError(PackFileName, err)) {
			if !placeholders[d.Pos.Line] || !strings.Contains(d.Message, "!!map") {
				problems = append(problems, d)
			}
		}
		if len(problems) > 0 {
			return nil, nil, problems
		}
	}

	declared := indexNode(PackFileName, pack, &root)
	normalizeOptions(written.Options, declared)
	problems := validateOptions(written.Options, declared)
	if len(problems) > 0 {
		return nil, nil, problems
	}
	values, problems := optionValues(use, written.Options, with, at, adder)
	overrides := make(map[string]diag.Pos)
	problems = append(problems, fillOptions(&root, "", written.Options, values, func(path, option string) {
		if _, given := with[option]; given {
			overrides[path] = adder.at(at + ".with." + option)
		}
	}, declared)...)
	if len(problems) > 0 {
		return nil, nil, problems
	}

	var filled packFile
	if err := root.Decode(&filled); err != nil {
		return nil, nil, inPack(decodeError(PackFileName, err))
	}
	if problems := ownTemplates(filled, indexNode(PackFileName, pack, &root)); len(problems) > 0 {
		return nil, nil, problems
	}
	cfg := &Config{Version: filled.Version, Partials: filled.Partials, Derive: filled.Derive, Generators: filled.Generators}
	pos := indexNode(PackFileName, pack, &root)
	pos.overrides = overrides
	if problems := cfg.renderPaths(values, funcs, adder.at(at), pos); len(problems) > 0 {
		return nil, nil, problems
	}
	cfg.normalize()
	cfg.recordPositions(pos)
	// Positions already name the pack, except those of options given in adder.yaml.
	problems = diag.From(cfg.validate(pos))
	for _, name := range slices.Sorted(maps.Keys(cfg.Generators)) {
		if cfg.Generators[name].Tests != nil {
			problems = append(problems, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at("generators." + name + ".tests"),
				Message: fmt.Sprintf("generator %q: a pack's generators have no tests of their own", name),
				Hint:    "golden files belong to a repository: test the generator under its use in adder.yaml, with tests: {" + name + ": [...]}",
			})
		}
	}
	if len(problems) > 0 {
		return nil, nil, problems
	}
	for i := range cfg.Derive.Outputs {
		cfg.Derive.Outputs[i].Pack, cfg.Derive.Outputs[i].Use = pack, use
	}
	return cfg, values, nil
}

// ownTemplates reports outputs and generator files in a pack.yaml that name a pack, since a pack renders only
// its own templates.
func ownTemplates(pf packFile, pos positions) diag.List {
	var errs diag.List
	invalid := func(at string) {
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     pos.at(at),
			Message: "a pack renders only its own templates",
			Hint:    "remove pack; template paths in pack.yaml are relative to the pack's root",
		})
	}
	for i, out := range pf.Derive.Outputs {
		if out.Pack != "" {
			invalid("derive.outputs." + strconv.Itoa(i) + ".pack")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(pf.Generators)) {
		for i, f := range pf.Generators[name].Files {
			if f.Pack != "" {
				invalid("generators." + name + ".files." + strconv.Itoa(i) + ".pack")
			}
		}
	}
	return errs
}

// optionLines returns the lines of the mappings that stand for an option's value.
func optionLines(n *yaml.Node) map[int]bool {
	lines := make(map[int]bool)
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if isOption(n) {
			lines[n.Line] = true
			return
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(n)
	return lines
}

func isOption(n *yaml.Node) bool {
	return n.Kind == yaml.MappingNode && len(n.Content) == 2 && n.Content[0].Value == optionKey
}

// fillOptions replaces each mapping that stands for an option with the option's value, calling filled with the
// path of each one replaced.
func fillOptions(n *yaml.Node, path string, options map[string]Option, values map[string]any, filled func(path, option string), pos positions) diag.List {
	switch {
	case isOption(n):
		name := n.Content[1].Value
		if _, ok := options[name]; !ok {
			return diag.List{diag.Errorf(diag.InvalidValue, pos.pos(n), "%s refers to option %q, which is not declared under options", path, name)}
		}
		*n = *valueNode(values[name], n.Line, n.Column)
		filled(path, name)
		return nil
	case n.Kind == yaml.DocumentNode:
		return fillOptions(n.Content[0], path, options, values, filled, pos)
	case n.Kind == yaml.MappingNode:
		var errs diag.List
		for i := 0; i+1 < len(n.Content); i += 2 {
			if child := joinPath(path, n.Content[i].Value); child != "options" {
				errs = append(errs, fillOptions(n.Content[i+1], child, options, values, filled, pos)...)
			}
		}
		return errs
	case n.Kind == yaml.SequenceNode:
		var errs diag.List
		for i, child := range n.Content {
			errs = append(errs, fillOptions(child, joinPath(path, strconv.Itoa(i)), options, values, filled, pos)...)
		}
		return errs
	}
	return nil
}

// valueNode returns an option's value as a YAML node at the given position.
func valueNode(v any, line, column int) *yaml.Node {
	switch v := v.(type) {
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v), Line: line, Column: column}
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Line: line, Column: column}
		for _, s := range v {
			seq.Content = append(seq.Content, valueNode(s, line, column))
		}
		return seq
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(v), Line: line, Column: column}
	}
}

// renderPaths renders the paths of outputs without each, which may use .Options and the template functions.
// Paths of outputs with each render for every item, with .Options too. A problem with the result is reported at
// use.
func (c *Config) renderPaths(values map[string]any, funcs template.FuncMap, use diag.Pos, pos positions) diag.List {
	var errs diag.List
	for i := range c.Derive.Outputs {
		out := &c.Derive.Outputs[i]
		if out.Each != "" || !strings.Contains(out.Path, "{{") {
			continue
		}
		at := "derive.outputs." + strconv.Itoa(i) + ".path"
		tmpl, err := template.New("path").Funcs(render.Funcs()).Funcs(funcs).Option("missingkey=error").Parse(out.Path)
		var b strings.Builder
		if err == nil {
			err = tmpl.Execute(&b, map[string]any{"Options": values})
		}
		if err != nil {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at),
				Message: fmt.Sprintf("output %d: path %q: %v", i, out.Path, err),
				Hint:    "the path of an output without each can only use .Options",
			})
			continue
		}
		out.Path = b.String()
		pos.overrides[at] = use
	}
	return errs
}

func normalizeOptions(options map[string]Option, pos positions) {
	for name, opt := range options {
		if opt.Type == "" {
			opt.Type = OptionString
		}
		if opt.Pattern.Regexp != nil {
			opt.whole = regexp.MustCompile(`^(?:` + opt.Pattern.String() + `)$`)
		}
		opt.Pos = pos.at("options." + name)
		options[name] = opt
	}
}

// validateOptions checks the options a pack declares.
func validateOptions(options map[string]Option, pos positions) diag.List {
	var errs diag.List
	for _, name := range slices.Sorted(maps.Keys(options)) {
		opt := options[name]
		at := "options." + name
		field := fmt.Sprintf("option %q", name)
		if !inputName.MatchString(name) {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidName,
				Pos:     opt.Pos,
				Message: field + ": name must be lowerCamelCase",
				Hint:    "templates read options as .Options.<name>, such as .Options.outDir",
			})
		}
		switch opt.Type {
		case OptionString, OptionList:
		case OptionBool:
			for _, setting := range []struct {
				key string
				set bool
			}{{"pattern", opt.Pattern.Regexp != nil}, {"values", opt.Values != nil}} {
				if setting.set {
					errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+"."+setting.key), "%s: %s does not apply to a bool option", field, setting.key))
				}
			}
		default:
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.InvalidValue,
				Pos:     pos.at(at + ".type"),
				Message: fmt.Sprintf("%s: unknown type %q", field, opt.Type),
				Hint:    "type is string, bool, or list",
			})
			continue
		}
		if opt.Default != nil {
			if _, err := opt.value(opt.Default); err != nil {
				errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".default"), "%s: default %v", field, err))
			}
		}
	}
	return errs
}

// optionValues returns the value of every option a pack declares, from the use's with or the option's default.
func optionValues(use string, options map[string]Option, with map[string]any, at string, pos positions) (map[string]any, diag.List) {
	var errs diag.List
	for _, key := range slices.Sorted(maps.Keys(with)) {
		if _, ok := options[key]; !ok {
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.UnknownField,
				Pos:     pos.at(at + ".with." + key),
				Message: fmt.Sprintf("pack use %q: unknown option %q", use, key),
				Hint:    optionsHint(options),
			})
		}
	}
	values := make(map[string]any, len(options))
	for _, name := range slices.Sorted(maps.Keys(options)) {
		opt := options[name]
		v, given := with[name]
		switch {
		case given:
		case opt.Default != nil:
			v = opt.Default
		case opt.Type == OptionBool:
			v = false
		default:
			errs = append(errs, &diag.Diagnostic{
				Code:    diag.MissingField,
				Pos:     pos.at(at),
				Message: fmt.Sprintf("pack use %q: option %q is required", use, name),
				Hint:    strings.TrimSpace("set it under with. " + opt.Help),
			})
			continue
		}
		value, err := opt.value(v)
		if err != nil {
			errs = append(errs, diag.Errorf(diag.InvalidValue, pos.at(at+".with."+name), "pack use %q: option %q %v", use, name, err))
			continue
		}
		values[name] = value
	}
	return values, errs
}

func optionsHint(options map[string]Option) string {
	if len(options) == 0 {
		return "the pack takes no options"
	}
	return "the pack's options: " + strings.Join(slices.Sorted(maps.Keys(options)), ", ")
}

// value checks v, decoded from YAML, against the option, and returns it as templates see it: a string, a bool,
// or a []string.
func (o Option) value(v any) (any, error) {
	switch o.Type {
	case OptionBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("is %v, want true or false", describe(v))
		}
		return b, nil
	case OptionList:
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("is %v, want a list of strings", describe(v))
		}
		out := make([]string, len(list))
		for i, elem := range list {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("value %d is %v, want a string", i, describe(elem))
			}
			if err := o.check(s); err != nil {
				return nil, fmt.Errorf("value %d %w", i, err)
			}
			out[i] = s
		}
		return out, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("is %v, want a string", describe(v))
		}
		return s, o.check(s)
	}
}

// check applies the option's pattern and values to s.
func (o Option) check(s string) error {
	switch {
	case o.whole != nil && !o.whole.MatchString(s):
		return fmt.Errorf("%q does not match the pattern %s", s, o.Pattern)
	case o.Values != nil && !slices.Contains(o.Values, s):
		return fmt.Errorf("%q is not one of %s", s, strings.Join(o.Values, ", "))
	}
	return nil
}

// describe names a decoded YAML value in a problem.
func describe(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case []any:
		return "a list"
	case map[string]any:
		return "a mapping"
	case nil:
		return "empty"
	default:
		return fmt.Sprint(v)
	}
}

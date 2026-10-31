package scaffold

import (
	"context"
	"maps"
	"slices"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
)

// Description is what a generator takes and creates, for adder describe.
type Description struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Inputs      []DescribedInput `json:"inputs"`
	Files       []DescribedFile  `json:"files"`
	Message     string           `json:"message"` // the message template as written
	Derive      bool             `json:"derive"`
	// Deprecated says what to use instead of the generator, when it is deprecated.
	Deprecated  string      `json:"deprecated,omitempty"`
	InputSchema InputSchema `json:"inputSchema"`

	gen  config.Generator
	data map[string]any // the scans, when an input refers to one
}

// DescribedInput is one input of a generator, with the values its scans allow now.
type DescribedInput struct {
	Name       string  `json:"name"`
	Flag       string  `json:"flag"`
	Type       string  `json:"type"`
	Help       string  `json:"help"`
	Required   bool    `json:"required"`
	Default    *string `json:"default,omitempty"`
	Pattern    string  `json:"pattern,omitempty"`
	ValuesFrom string  `json:"valuesFrom,omitempty"`
	Unique     string  `json:"unique,omitempty"`
	// Values are the allowed values, from values or valuesFrom. Nil means any value.
	Values []string `json:"values"`
	// Existing are the values unique rejects. Nil means the input has no unique.
	Existing []string `json:"existing"`
	// Deprecated says what to use instead of the input, when it is deprecated.
	Deprecated string `json:"deprecated,omitempty"`
}

// DescribedFile is one file a generator creates.
type DescribedFile struct {
	Template string `json:"template"`
	Path     string `json:"path"`
	When     string `json:"when,omitempty"`
	Unless   string `json:"unless,omitempty"`
	IfExists string `json:"ifExists"`
}

// InputSchema is a JSON Schema for the object adder new --inputs accepts, the shape MCP uses for tool inputs.
type InputSchema struct {
	Schema               string              `json:"$schema"`
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties"`
	Required             []string            `json:"required"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

// Property is the schema of one input.
type Property struct {
	Type        string    `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
	Default     any       `json:"default,omitempty"`
	Pattern     string    `json:"pattern,omitempty"`
	Enum        []string  `json:"enum,omitzero"`
	Not         *Property `json:"not,omitempty"`
	Deprecated  bool      `json:"deprecated,omitempty"`
}

// Describe returns what the generator called name takes and creates. Values from valuesFrom and unique are
// computed from the scans of src, so they are current.
func Describe(ctx context.Context, cfg *config.Config, src derive.Sources, name string) (*Description, error) {
	gen, ok := cfg.Generators[name]
	if !ok {
		return nil, unknownGenerator(cfg, name)
	}
	var data map[string]any
	if slices.ContainsFunc(slices.Collect(maps.Values(gen.Inputs)), func(in config.Input) bool { return in.ValuesFrom != "" || in.Unique != "" }) {
		var err error
		if _, data, err = derive.Prepare(ctx, cfg, src, gen.Use); err != nil {
			return nil, err
		}
	}

	d := &Description{
		Name:        name,
		Description: gen.Description,
		Inputs:      []DescribedInput{},
		Files:       []DescribedFile{},
		Message:     gen.Message,
		Derive:      gen.Derive == nil || *gen.Derive,
		Deprecated:  gen.Deprecated,
		gen:         gen,
		data:        data,
		InputSchema: InputSchema{
			Schema:     "https://json-schema.org/draft/2020-12/schema",
			Type:       "object",
			Properties: make(map[string]Property, len(gen.Inputs)),
			Required:   []string{},
		},
	}
	for _, input := range slices.Sorted(maps.Keys(gen.Inputs)) {
		in := gen.Inputs[input]
		described, err := describeInput(input, in, data)
		if err != nil {
			return nil, err
		}
		d.Inputs = append(d.Inputs, described)
		d.InputSchema.Properties[input] = property(described, in)
		if described.Required {
			d.InputSchema.Required = append(d.InputSchema.Required, input)
		}
	}
	for _, f := range gen.Files {
		d.Files = append(d.Files, DescribedFile{Template: f.Template, Path: f.Path, When: f.When, Unless: f.Unless, IfExists: f.IfExists})
	}
	return d, nil
}

// Check reports whether value is valid for the string input called input, as adder new checks it.
func (d *Description) Check(input, value string) error {
	return check("--"+config.Flag(input), value, d.gen.Inputs[input], d.data)
}

func describeInput(name string, in config.Input, data map[string]any) (DescribedInput, error) {
	d := DescribedInput{
		Name:       name,
		Flag:       "--" + config.Flag(name),
		Type:       in.Type,
		Help:       in.Help,
		Required:   in.Required(),
		Default:    in.Default,
		ValuesFrom: in.ValuesFrom,
		Unique:     in.Unique,
		Values:     in.Values,
		Deprecated: in.Deprecated,
	}
	if in.Pattern.Regexp != nil {
		d.Pattern = in.Pattern.String()
	}
	var err error
	if in.ValuesFrom != "" {
		if d.Values, err = scanValues(in.ValuesFrom, in, data); err != nil {
			return DescribedInput{}, err
		}
		d.Values = nonNil(d.Values)
	}
	if in.Unique != "" {
		if d.Existing, err = scanValues(in.Unique, in, data); err != nil {
			return DescribedInput{}, err
		}
		d.Existing = nonNil(d.Existing)
	}
	return d, nil
}

func property(d DescribedInput, in config.Input) Property {
	if in.Type == config.InputBool {
		return Property{Type: "boolean", Description: d.Help, Default: false, Deprecated: in.Deprecated != ""}
	}
	p := Property{Type: "string", Description: d.Help, Pattern: in.Anchored(), Enum: d.Values, Deprecated: in.Deprecated != ""}
	if d.Default != nil {
		p.Default = *d.Default
	}
	if len(d.Existing) > 0 {
		p.Not = &Property{Enum: d.Existing}
	}
	return p
}

// nonNil returns s, or an empty slice for nil, so that JSON tells "no values" from "any value".
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

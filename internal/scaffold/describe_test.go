package scaffold

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/diag"
)

// describeFixture loads the drivers fixture with two more inputs, so every kind of restriction is described.
func describeFixture(t *testing.T) (*config.Config, derive.Sources) {
	t.Helper()
	root, cfg := copyRepo(t)
	gen := cfg.Generators["driver"]
	gen.Inputs["base"] = config.Input{Type: config.InputString, Help: "Driver to copy", Default: new("local"), ValuesFrom: "Drivers.Package"}
	gen.Inputs["tier"] = config.Input{Type: config.InputString, Values: []string{"fast", "slow"}, Default: new("fast")}
	src, closeSources, err := derive.Open(cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeSources)
	return cfg, src
}

func TestDescribe(t *testing.T) {
	cfg, src := describeFixture(t)
	d, err := Describe(t.Context(), cfg, src, "driver")
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	byName := make(map[string]DescribedInput)
	for _, in := range d.Inputs {
		names = append(names, in.Name)
		byName[in.Name] = in
	}
	if want := []string{"base", "cache", "name", "summary", "tier"}; !slices.Equal(names, want) {
		t.Fatalf("inputs = %v, want %v", names, want)
	}
	tests := []struct {
		input        string
		wantFlag     string
		wantRequired bool
		wantValues   []string
		wantExisting []string
	}{
		{input: "base", wantFlag: "--base", wantValues: []string{"local", "s3"}},
		{input: "cache", wantFlag: "--cache"},
		{input: "name", wantFlag: "--name", wantRequired: true, wantExisting: []string{"local", "s3"}},
		{input: "summary", wantFlag: "--summary"},
		{input: "tier", wantFlag: "--tier", wantValues: []string{"fast", "slow"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := byName[tt.input]
			if got.Flag != tt.wantFlag || got.Required != tt.wantRequired || !slices.Equal(got.Values, tt.wantValues) || !slices.Equal(got.Existing, tt.wantExisting) {
				t.Errorf("input = %+v, want flag %s, required %v, values %v, existing %v", got, tt.wantFlag, tt.wantRequired, tt.wantValues, tt.wantExisting)
			}
		})
	}
	if len(d.Files) != 3 || d.Files[1].When != "cache" || d.Files[2].IfExists != config.IfExistsSkip || !d.Derive {
		t.Errorf("files = %+v, derive = %v", d.Files, d.Derive)
	}
	if !slices.Equal(d.InputSchema.Required, []string{"name"}) {
		t.Errorf("required = %v, want [name]", d.InputSchema.Required)
	}
}

// TestInputSchema checks that the schema is valid JSON Schema and accepts exactly the inputs adder new accepts.
func TestInputSchema(t *testing.T) {
	cfg, src := describeFixture(t)
	d, err := Describe(t.Context(), cfg, src, "driver")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(d.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("inputs.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("inputs.json")
	if err != nil {
		t.Fatalf("the input schema is not valid JSON Schema: %v\n%s", err, encoded)
	}
	_, data, err := derive.Prepare(t.Context(), cfg, src)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		inputs string
		want   bool
	}{
		{name: "required only", inputs: `{"name": "gcs"}`, want: true},
		{name: "every input", inputs: `{"name": "gcs", "summary": "s", "cache": true, "base": "s3", "tier": "slow"}`, want: true},
		{name: "missing a required input", inputs: `{}`},
		{name: "a value that already exists", inputs: `{"name": "s3"}`},
		{name: "a value outside the pattern", inputs: `{"name": "Bad"}`},
		{name: "a pattern matching only part of the value", inputs: `{"name": "gcs!"}`},
		{name: "a value not from the scan", inputs: `{"name": "gcs", "base": "gcs"}`},
		{name: "a value not listed", inputs: `{"name": "gcs", "tier": "medium"}`},
		{name: "a bool that is a string", inputs: `{"name": "gcs", "cache": "yes"}`},
		{name: "a string that is a number", inputs: `{"name": 5}`},
		{name: "an unknown input", inputs: `{"name": "gcs", "color": "red"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inputs map[string]any
			if err := json.Unmarshal([]byte(tt.inputs), &inputs); err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(tt.inputs)))
			if err != nil {
				t.Fatal(err)
			}
			schemaOK := schema.Validate(instance) == nil
			_, err = resolve(cfg.Generators["driver"], inputs, data)
			adderOK := err == nil
			if schemaOK != tt.want || adderOK != tt.want {
				t.Errorf("schema accepts %s: %v, adder accepts it: %v (%v), want %v", tt.inputs, schemaOK, adderOK, err, tt.want)
			}
		})
	}
}

func TestDescribeErrors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(cfg *config.Config)
		generator string
		wantCode  diag.Code
	}{
		{name: "unknown generator", generator: "drivr", wantCode: diag.UnknownGenerator},
		{
			name: "a scan that fails",
			setup: func(cfg *config.Config) {
				s := cfg.Derive.Scans["Drivers"]
				s.Dirs = []string{"missing/*"}
				cfg.Derive.Scans["Drivers"] = s
			},
			generator: "driver",
			wantCode:  diag.NoMatch,
		},
		{
			name: "a reference to a field items do not have",
			setup: func(cfg *config.Config) {
				in := cfg.Generators["driver"].Inputs["name"]
				in.Unique = "Drivers.Nope"
				cfg.Generators["driver"].Inputs["name"] = in
			},
			generator: "driver",
			wantCode:  diag.InvalidValue,
		},
		{
			name: "a valuesFrom reference to a field items do not have",
			setup: func(cfg *config.Config) {
				in := cfg.Generators["driver"].Inputs["summary"]
				in.ValuesFrom = "Drivers.Nope"
				cfg.Generators["driver"].Inputs["summary"] = in
			},
			generator: "driver",
			wantCode:  diag.InvalidValue,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := copyRepo(t)
			if tt.setup != nil {
				tt.setup(cfg)
			}
			src, closeSources, err := derive.Open(cfg, root)
			if err != nil {
				t.Fatal(err)
			}
			defer closeSources()
			_, err = Describe(t.Context(), cfg, src, tt.generator)
			if list := diag.From(err); len(list) == 0 || list[0].Code != tt.wantCode {
				t.Errorf("Describe() error = %v, want %s", err, tt.wantCode)
			}
		})
	}
}

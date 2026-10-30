package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletion(t *testing.T) {
	const withValues = `version: 1
derive:
  scans:
    Drivers: {dirs: ["drivers/*"], funcs: {name: ^New$}}
  outputs:
    - {template: templates/drivers.go.tmpl, path: drivers/drivers.gen.go}
generators:
  copy:
    description: Copy a driver
    inputs:
      from: {valuesFrom: Drivers.Package}
      tier: {values: [fast, slow], default: fast}
      plain: {type: bool}
    files:
      - {template: templates/new/README.md.tmpl, path: x.md}
`
	tests := []struct {
		name   string
		config string // replaces adder.yaml when set
		chdir  string // relative to root; "-" means a directory without adder.yaml
		words  []string
		// want is every completion, one per line, then the directive.
		want string
	}{
		{name: "generator names", words: []string{"new", ""}, want: "driver\tAdd a storage driver\n:4\n"},
		{name: "generator names by prefix", words: []string{"new", "x"}, want: ":4\n"},
		{name: "generator flags", words: []string{"new", "driver", "--na"}, want: "--name\tDriver name, used as the package name\n:4\n"},
		{name: "free text", words: []string{"new", "driver", "--summary", ""}, want: ":4\n"},
		{name: "no more arguments", words: []string{"new", "driver", ""}, want: ":4\n"},
		{name: "an unknown generator", words: []string{"new", "nope", "--na"}, want: ":4\n"},
		{name: "values", config: withValues, words: []string{"new", "copy", "--tier", ""}, want: "fast\nslow\n:4\n"},
		{name: "values from a scan", config: withValues, words: []string{"new", "copy", "--from", "s"}, want: "s3\n:4\n"},
		{name: "values after an equals sign", config: withValues, words: []string{"new", "copy", "--from="}, want: "--from=local\n--from=s3\n:4\n"},
		{name: "a bool input has no values", config: withValues, words: []string{"new", "copy", "--plain="}, want: ":4\n"},
		{name: "an inputs file", words: []string{"new", "driver", "--inputs", ""}, want: "json\n:8\n"},
		{name: "global flags before the generator", words: []string{"new", "--root", ".", ""}, want: "driver\tAdd a storage driver\n:4\n"},
		{name: "describe", words: []string{"describe", ""}, want: "driver\tAdd a storage driver\n:4\n"},
		{name: "describe takes one generator", words: []string{"describe", "driver", ""}, want: ":4\n"},
		{name: "test skips generators already named", words: []string{"test", "driver", ""}, want: ":4\n"},
		{name: "explain", words: []string{"explain", ""}, want: "drivers/drivers.gen.go\n:4\n"},
		{name: "explain from a subdirectory", chdir: "drivers/s3", words: []string{"explain", "../"}, want: "../drivers.gen.go\n:4\n"},
		{name: "explain takes one file", words: []string{"explain", "a", ""}, want: ":4\n"},
		{name: "without a configuration", chdir: "-", words: []string{"new", ""}, want: ":4\n"},
		{name: "explain without a configuration", chdir: "-", words: []string{"explain", ""}, want: ":0\n"},
		{name: "describe without a configuration", chdir: "-", words: []string{"describe", ""}, want: ":4\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyDir(t, filepath.Join("..", "scaffold", "testdata", "drivers", "repo"))
			if tt.config != "" {
				writeFile(t, filepath.Join(root, "adder.yaml"), tt.config)
			}
			if code, _, errOut := run(t.Context(), "--root", root, "derive"); code != ExitOK {
				t.Fatal(errOut)
			}
			switch tt.chdir {
			case "":
				t.Chdir(root)
			case "-":
				t.Chdir(t.TempDir())
			default:
				t.Chdir(filepath.Join(root, filepath.FromSlash(tt.chdir)))
			}

			_, stdout, _ := run(t.Context(), append([]string{"__complete"}, tt.words...)...)

			if stdout != tt.want {
				t.Errorf("completions for %q =\n%s\nwant\n%s", strings.Join(tt.words, " "), stdout, tt.want)
			}
		})
	}
}

package config

import (
	"bytes"
	"os"
	"testing"

	"typhonhill.dev/adder/internal/diag"
)

// FuzzParse checks that no input panics and that every problem points inside the input.
func FuzzParse(f *testing.F) {
	for _, path := range []string{"../derive/testdata/basic/repo/adder.yaml", "../scaffold/testdata/drivers/repo/adder.yaml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	for _, seed := range []string{
		"",
		"version: 1\n",
		"version: 2\n",
		"version: 1\nderive:\n  outputs:\n    - template: x.tmpl\n      paht: out.go\n",
		"version: 1\nvars:\n  1x: y\npartials: [\"**\"]\n",
		"version: 1\ngenerators:\n  Bad_Name:\n    inputs:\n      x: {type: bool, default: a}\n",
		"a: &a [*a]\n",
		"version: [1\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// YAML ends lines at \n, \r\n, or a lone \r.
		normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		lines := bytes.Count(normalized, []byte("\n")) + bytes.Count(normalized, []byte("\r")) + 1
		_, err := Parse(FileName, data, nil)
		for _, d := range diag.From(err) {
			if d.Message == "" {
				t.Errorf("problem %#v has no message", d)
			}
			if d.Pos.File != FileName || d.Pos.Line < 0 || d.Pos.Line > lines || d.Pos.Column < 0 {
				t.Errorf("problem %q is at %v, outside the %d-line input", d.Message, d.Pos, lines)
			}
		}
	})
}

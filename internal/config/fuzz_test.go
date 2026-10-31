package config

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

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
		lines := yamlLines(data)
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

// yamlLines counts the lines of data as YAML does: in UTF-16 after a byte order mark, and ending lines at \n,
// \r\n, a lone \r, and the Unicode line breaks NEL, LS, and PS.
func yamlLines(data []byte) int {
	text := string(data)
	for bom, order := range map[string]binary.ByteOrder{"\xfe\xff": binary.BigEndian, "\xff\xfe": binary.LittleEndian} {
		if rest, ok := bytes.CutPrefix(data, []byte(bom)); ok {
			units := make([]uint16, len(rest)/2)
			for i := range units {
				units[i] = order.Uint16(rest[2*i:])
			}
			text = string(utf16.Decode(units))
		}
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := 1
	for _, br := range []string{"\n", "\r", "\u0085", "\u2028", "\u2029"} {
		lines += strings.Count(text, br)
	}
	return lines
}

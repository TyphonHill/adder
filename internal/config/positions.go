package config

import (
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/TyphonHill/adder/internal/diag"
)

// positions maps dotted field paths, such as "derive.outputs.0.path", to where they appear in a file.
type positions struct {
	file   string
	byPath map[string]diag.Pos
}

func indexPositions(file string, data []byte) positions {
	idx := positions{file: file, byPath: make(map[string]diag.Pos)}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return idx
	}

	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		if _, ok := idx.byPath[path]; !ok {
			idx.byPath[path] = idx.pos(n)
		}
		switch n.Kind {
		case yaml.DocumentNode:
			for _, child := range n.Content {
				walk(child, path)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, value := n.Content[i], n.Content[i+1]
				child := joinPath(path, key.Value)
				idx.byPath[child] = idx.pos(key)
				walk(value, child)
			}
		case yaml.SequenceNode:
			for i, child := range n.Content {
				walk(child, joinPath(path, strconv.Itoa(i)))
			}
		}
	}
	walk(&root, "")
	return idx
}

func (p positions) pos(n *yaml.Node) diag.Pos {
	return diag.Pos{File: p.file, Line: n.Line, Column: n.Column}
}

// has reports whether path appears in the file.
func (p positions) has(path string) bool {
	_, ok := p.byPath[path]
	return ok
}

// at returns the position of path, or of its nearest ancestor when path itself is absent.
func (p positions) at(path string) diag.Pos {
	for {
		if pos, ok := p.byPath[path]; ok {
			return pos
		}
		if path == "" {
			return diag.Pos{File: p.file}
		}
		i := strings.LastIndexByte(path, '.')
		if i < 0 {
			path = ""
		} else {
			path = path[:i]
		}
	}
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

// knownFields lists the YAML keys of each configuration type, keyed by the type name the decoder reports.
var knownFields = fieldsOf(Config{}, Derive{}, Scan{}, FuncMatch{}, TypeMatch{}, ValueMatch{}, FileMatch{}, Output{}, Generator{}, Input{}, File{}, GeneratorTest{})

func fieldsOf(values ...any) map[string][]string {
	fields := make(map[string][]string, len(values))
	for _, v := range values {
		t := reflect.TypeOf(v)
		for f := range t.Fields() {
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if name != "" && name != "-" {
				fields[t.String()] = append(fields[t.String()], name)
			}
		}
	}
	return fields
}

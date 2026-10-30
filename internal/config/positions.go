package config

import (
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"typhonhill.dev/adder/internal/diag"
)

// positions maps dotted field paths, such as "derive.outputs.0.path", to where they appear in a file.
type positions struct {
	file   string
	pack   string // the pack the file belongs to, if any
	byPath map[string]diag.Pos
	// overrides holds positions elsewhere for paths whose values came from there, such as a pack's options.
	overrides map[string]diag.Pos
}

func indexPositions(file string, data []byte) positions {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return positions{file: file, byPath: make(map[string]diag.Pos)}
	}
	return indexNode(file, "", &root)
}

// indexNode indexes the positions in a parsed file, which belongs to pack when it is not empty.
func indexNode(file, pack string, root *yaml.Node) positions {
	idx := positions{file: file, pack: pack, byPath: make(map[string]diag.Pos), overrides: make(map[string]diag.Pos)}

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
	walk(root, "")
	return idx
}

func (p positions) pos(n *yaml.Node) diag.Pos {
	return diag.Pos{Pack: p.pack, File: p.file, Line: n.Line, Column: n.Column}
}

// has reports whether path appears in the file.
func (p positions) has(path string) bool {
	_, ok := p.byPath[path]
	return ok
}

// at returns the position of path, or of its nearest ancestor when path itself is absent. A path at or below an
// override takes the override's position.
func (p positions) at(path string) diag.Pos {
	for up := path; len(p.overrides) > 0; up = parent(up) {
		if pos, ok := p.overrides[up]; ok {
			return pos
		}
		if up == "" {
			break
		}
	}
	for {
		if pos, ok := p.byPath[path]; ok {
			return pos
		}
		if path == "" {
			return diag.Pos{Pack: p.pack, File: p.file}
		}
		path = parent(path)
	}
}

func parent(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return ""
	}
	return path[:i]
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

// knownFields lists the YAML keys of each configuration type, keyed by the type name the decoder reports.
var knownFields = fieldsOf(Config{}, Derive{}, Scan{}, FuncMatch{}, TypeMatch{}, ValueMatch{}, FileMatch{}, Output{}, Generator{}, Input{}, File{}, GeneratorTest{}, Use{}, Option{}, packFile{}, ScannerMatch{})

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

package render

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/TyphonHill/adder/internal/textcase"
)

// Funcs returns the functions available to every template. Names follow Sprout
// (github.com/go-sprout/sprout) where it has an equivalent. Every function is deterministic.
func Funcs() template.FuncMap {
	return template.FuncMap{
		"toLower":        strings.ToLower,
		"toUpper":        strings.ToUpper,
		"toCamelCase":    textcase.Camel,
		"toPascalCase":   textcase.Pascal,
		"toSnakeCase":    textcase.Snake,
		"toKebabCase":    textcase.Kebab,
		"toConstantCase": textcase.Constant,
		"toGoCamelCase":  textcase.GoCamel,
		"toGoPascalCase": textcase.GoPascal,
		"fieldValues":    FieldValues,
		"groupBy":        groupBy,
		"sortBy":         sortBy,
		"uniq":           uniq,
		"sortAlpha":      sortAlpha,
		"join":           join,
		"trimPrefix":     func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
		"trimSuffix":     func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
		"replace":        func(old, replacement, s string) string { return strings.ReplaceAll(s, old, replacement) },
		"quote":          quote,
		"indent":         indent,
		"nindent":        nindent,
		"plural":         plural,
	}
}

// FieldValues returns the named string field of every struct in list.
func FieldValues(field string, list any) ([]string, error) {
	return stringFields("fieldValues", field, list)
}

// stringFields returns the named string field of every struct in list, for the template function fn.
func stringFields(fn, field string, list any) ([]string, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, fmt.Errorf("%s: want a list, got %T", fn, list)
	}

	out := make([]string, 0, v.Len())
	for i := range v.Len() {
		elem := reflect.Indirect(v.Index(i))
		if elem.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%s: element %d is %s, want a struct", fn, i, elem.Kind())
		}
		f := elem.FieldByName(field)
		if !f.IsValid() || f.Kind() != reflect.String {
			return nil, fmt.Errorf("%s: %s has no string field %q", fn, elem.Type(), field)
		}
		out = append(out, f.String())
	}
	return out, nil
}

// Group is the items of a list that share a key, as groupBy returns them.
type Group struct {
	Key   string
	Items []any
}

// groupBy groups the structs in list by their string field, sorted by key. Items keep their order in list.
func groupBy(field string, list any) ([]Group, error) {
	keys, err := stringFields("groupBy", field, list)
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(list)
	groups := make([]Group, 0)
	index := make(map[string]int)
	for i, key := range keys {
		j, ok := index[key]
		if !ok {
			j = len(groups)
			index[key] = j
			groups = append(groups, Group{Key: key})
		}
		groups[j].Items = append(groups[j].Items, v.Index(i).Interface())
	}
	slices.SortStableFunc(groups, func(a, b Group) int { return strings.Compare(a.Key, b.Key) })
	return groups, nil
}

// sortBy returns a copy of list sorted by the named field of its structs, a string or a number. Items with equal
// values keep their order, so sortBy "Iota" puts an enum in declaration order.
func sortBy(field string, list any) (any, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, fmt.Errorf("sortBy: want a list, got %T", list)
	}
	type keyed struct {
		elem reflect.Value
		key  reflect.Value
	}
	elems := make([]keyed, v.Len())
	for i := range v.Len() {
		elem := reflect.Indirect(v.Index(i))
		if elem.Kind() != reflect.Struct {
			return nil, fmt.Errorf("sortBy: element %d is %s, want a struct", i, elem.Kind())
		}
		key := elem.FieldByName(field)
		if !key.IsValid() || !sortable(key.Kind()) {
			return nil, fmt.Errorf("sortBy: %s has no string or number field %q", elem.Type(), field)
		}
		if i > 0 && key.Kind() != elems[0].key.Kind() {
			return nil, fmt.Errorf("sortBy: field %q is %s in element %d but %s in element 0", field, key.Kind(), i, elems[0].key.Kind())
		}
		elems[i] = keyed{v.Index(i), key}
	}
	slices.SortStableFunc(elems, func(a, b keyed) int { return compareValues(a.key, b.key) })
	out := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), len(elems), len(elems))
	for i, e := range elems {
		out.Index(i).Set(e.elem)
	}
	return out.Interface(), nil
}

func sortable(k reflect.Kind) bool {
	return k == reflect.String || reflect.Int <= k && k <= reflect.Float64
}

// compareValues orders two values of the same sortable kind.
func compareValues(a, b reflect.Value) int {
	switch {
	case a.Kind() == reflect.String:
		return strings.Compare(a.String(), b.String())
	case a.CanInt():
		return cmp.Compare(a.Int(), b.Int())
	case a.CanUint():
		return cmp.Compare(a.Uint(), b.Uint())
	default:
		return cmp.Compare(a.Float(), b.Float())
	}
}

// join joins the elements of list with sep, writing each as fmt.Sprint does.
func join(sep string, list any) (string, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return "", fmt.Errorf("join: want a list, got %T", list)
	}
	parts := make([]string, v.Len())
	for i := range v.Len() {
		parts[i] = fmt.Sprint(v.Index(i).Interface())
	}
	return strings.Join(parts, sep), nil
}

// quote returns v as a quoted Go string. Unlike Sprout's quote it takes exactly one value, so a template written
// for Sprout fails rather than renders differently.
func quote(v any) string {
	if v == nil {
		return ""
	}
	return strconv.Quote(fmt.Sprint(v))
}

// indent indents every line of s by n spaces.
func indent(n int, s string) (string, error) {
	if n < 0 {
		return "", fmt.Errorf("indent: want a number of spaces, got %d", n)
	}
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad), nil
}

// nindent is indent after a newline, so a block can start on its own line.
func nindent(n int, s string) (string, error) {
	indented, err := indent(n, s)
	return "\n" + indented, err
}

// plural returns one when count is 1 and many otherwise.
func plural(one, many string, count int) string {
	if count == 1 {
		return one
	}
	return many
}

// uniq drops repeated values, keeping the first occurrence of each.
func uniq(list []string) []string {
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func sortAlpha(list []string) []string {
	return slices.Sorted(slices.Values(list))
}

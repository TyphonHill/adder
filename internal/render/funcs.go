package render

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"typhonhill.dev/adder/internal/textcase"
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

// builtins are the functions and keywords text/template defines itself.
var builtins = []string{
	"and", "block", "break", "call", "continue", "define", "else", "end", "eq", "false", "ge", "gt", "html", "if",
	"index", "js", "le", "len", "lt", "ne", "nil", "not", "or", "print", "printf", "println", "range", "slice",
	"template", "true", "urlquery", "with",
}

var errorType = reflect.TypeFor[error]()

// CheckFunc reports why fn cannot be added to the template functions as name: a name that is not an identifier
// or is already built in, or a value text/template cannot call.
func CheckFunc(name string, fn any) error {
	if name == "" {
		return errors.New("a function has no name")
	}
	if _, builtin := Funcs()[name]; builtin || slices.Contains(builtins, name) {
		return fmt.Errorf("function %q is built in", name)
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return fmt.Errorf("function name %q is not an identifier", name)
		}
	}
	t := reflect.TypeOf(fn)
	switch {
	case t == nil || t.Kind() != reflect.Func || reflect.ValueOf(fn).IsNil():
		return fmt.Errorf("function %q is %T, not a function", name, fn)
	case t.NumOut() == 1, t.NumOut() == 2 && t.Out(1) == errorType:
		return nil
	default:
		return fmt.Errorf("function %q must return one value, or a value and an error", name)
	}
}

// FieldValues returns the named string field of every struct or map in list.
func FieldValues(field string, list any) ([]string, error) {
	return stringFields("fieldValues", field, list)
}

// stringFields returns the named string field of every struct or map in list, for the template function fn.
func stringFields(fn, field string, list any) ([]string, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, fmt.Errorf("%s: want a list, got %T", fn, list)
	}

	out := make([]string, 0, v.Len())
	for i := range v.Len() {
		f, elem, err := fieldOf(fn, v.Index(i), i, field)
		if err != nil {
			return nil, err
		}
		if f.Kind() != reflect.String {
			return nil, fmt.Errorf("%s: %s has no string field %q", fn, elem.Type(), field)
		}
		out = append(out, f.String())
	}
	return out, nil
}

// fieldOf returns the named field of the element at index i of a list, a struct or a map with string keys as
// custom scanners return, and the element itself.
func fieldOf(fn string, v reflect.Value, i int, field string) (reflect.Value, reflect.Value, error) {
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, v, fmt.Errorf("%s: element %d is nil", fn, i)
		}
		v = v.Elem()
	}
	var f reflect.Value
	switch {
	case v.Kind() == reflect.Struct:
		f = v.FieldByName(field)
	case v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String:
		f = v.MapIndex(reflect.ValueOf(field).Convert(v.Type().Key()))
		if f.IsValid() && f.Kind() == reflect.Interface {
			f = f.Elem()
		}
	default:
		return reflect.Value{}, v, fmt.Errorf("%s: element %d is %s, want a struct or a map", fn, i, v.Kind())
	}
	if !f.IsValid() {
		return reflect.Value{}, v, fmt.Errorf("%s: %s has no field %q", fn, v.Type(), field)
	}
	return f, v, nil
}

// Group is the items of a list that share a key, as groupBy returns them.
type Group struct {
	Key   string
	Items []any
}

// groupBy groups the structs or maps in list by their string field, sorted by key. Items keep their order in list.
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

// sortBy returns a copy of list sorted by the named field of its structs or maps, a string or a number. Items with equal
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
		key, elem, err := fieldOf("sortBy", v.Index(i), i, field)
		if err != nil {
			return nil, err
		}
		if !sortable(key.Kind()) {
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

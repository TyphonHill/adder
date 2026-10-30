package render

import (
	"fmt"
	"reflect"
	"slices"
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
		"uniq":           uniq,
		"sortAlpha":      sortAlpha,
	}
}

// FieldValues returns the named string field of every struct in list.
func FieldValues(field string, list any) ([]string, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, fmt.Errorf("fieldValues: want a list, got %T", list)
	}

	out := make([]string, 0, v.Len())
	for i := range v.Len() {
		elem := reflect.Indirect(v.Index(i))
		if elem.Kind() != reflect.Struct {
			return nil, fmt.Errorf("fieldValues: element %d is %s, want a struct", i, elem.Kind())
		}
		f := elem.FieldByName(field)
		if !f.IsValid() || f.Kind() != reflect.String {
			return nil, fmt.Errorf("fieldValues: %s has no string field %q", elem.Type(), field)
		}
		out = append(out, f.String())
	}
	return out, nil
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

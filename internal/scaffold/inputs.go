package scaffold

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/render"
	"typhonhill.dev/adder/internal/scan"
)

// resolve checks the given inputs against the generator's declarations and fills in defaults. Data is the
// template data, whose scans supply valuesFrom and unique.
func resolve(gen config.Generator, given map[string]any, data map[string]any) (map[string]any, error) {
	var problems diag.List
	for _, name := range slices.Sorted(maps.Keys(given)) {
		if _, ok := gen.Inputs[name]; !ok {
			problems = append(problems, &diag.Diagnostic{
				Code:    diag.InvalidInput,
				Message: fmt.Sprintf("unknown input %q", name),
				Hint:    inputsHint(gen),
			})
		}
	}

	values := make(map[string]any, len(gen.Inputs))
	for _, name := range slices.Sorted(maps.Keys(gen.Inputs)) {
		in := gen.Inputs[name]
		flag := "--" + config.Flag(name)
		value, ok := given[name]

		if in.Type == config.InputBool {
			b, isBool := value.(bool)
			if ok && !isBool {
				problems = append(problems, diag.Errorf(diag.InvalidInput, diag.Pos{}, "%s must be true or false, not %v", flag, value))
			}
			values[name] = b
			continue
		}

		if !ok {
			if in.Default == nil {
				problems = append(problems, &diag.Diagnostic{Code: diag.MissingInput, Message: flag + " is required", Hint: in.Help})
				continue
			}
			value = *in.Default
		}
		s, isString := value.(string)
		if !isString {
			problems = append(problems, diag.Errorf(diag.InvalidInput, diag.Pos{}, "%s must be a string, not %v", flag, value))
			continue
		}
		if err := check(flag, s, in, data); err != nil {
			problems = append(problems, diag.From(err)...)
			continue
		}
		values[name] = s
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// check reports a string value that the input's restrictions reject.
func check(flag, value string, in config.Input, data map[string]any) error {
	invalid := func(format string, args ...any) error {
		return &diag.Diagnostic{Code: diag.InvalidInput, Message: fmt.Sprintf("%s %q ", flag, value) + fmt.Sprintf(format, args...), Hint: in.Help}
	}
	if !in.MatchesPattern(value) {
		return invalid("does not match %s", in.Pattern)
	}
	if in.Values != nil && !slices.Contains(in.Values, value) {
		return invalid("is not one of: %s", strings.Join(in.Values, ", "))
	}
	if in.ValuesFrom != "" {
		allowed, err := scanValues(in.ValuesFrom, in, data)
		if err != nil {
			return err
		}
		if !slices.Contains(allowed, value) {
			if len(allowed) == 0 {
				return invalid("is not allowed: %s has no values", in.ValuesFrom)
			}
			return invalid("is not one of the %s values: %s", in.ValuesFrom, strings.Join(allowed, ", "))
		}
	}
	if in.Unique != "" {
		existing, err := scanValues(in.Unique, in, data)
		if err != nil {
			return err
		}
		if slices.Contains(existing, value) {
			return invalid("already exists in %s", in.Unique)
		}
	}
	return nil
}

// scanValues returns the values of a Scan.Field reference, sorted and without repeats.
func scanValues(ref string, in config.Input, data map[string]any) ([]string, error) {
	scanName, field, _ := strings.Cut(ref, ".")
	// A custom scanner's items have fields of their own, which FieldValues checks.
	if _, builtin := data[scanName].([]scan.Item); builtin && !slices.Contains(itemFields, field) {
		return nil, &diag.Diagnostic{
			Code:    diag.InvalidValue,
			Pos:     in.Pos,
			Message: fmt.Sprintf("%s: scan items have no string field %q", ref, field),
			Hint:    "string fields: " + strings.Join(itemFields, ", "),
		}
	}
	values, err := render.FieldValues(field, data[scanName])
	if err != nil {
		return nil, diag.Wrap(diag.InvalidValue, in.Pos, err)
	}
	slices.Sort(values)
	return slices.Compact(values), nil
}

// itemFields are the string fields of scan items, which valuesFrom and unique can refer to.
var itemFields = func() []string {
	var names []string
	for f := range reflect.TypeFor[scan.Item]().Fields() {
		if f.Type.Kind() == reflect.String {
			names = append(names, f.Name)
		}
	}
	slices.Sort(names)
	return names
}()

func inputsHint(gen config.Generator) string {
	if len(gen.Inputs) == 0 {
		return "this generator takes no inputs"
	}
	return "inputs: " + strings.Join(slices.Sorted(maps.Keys(gen.Inputs)), ", ")
}

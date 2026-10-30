package render

import "testing"

func TestCaseFuncs(t *testing.T) {
	const in = "HTTPServer_id"
	funcs := Funcs()
	tests := []struct{ fn, want string }{
		{"toLower", "httpserver_id"},
		{"toUpper", "HTTPSERVER_ID"},
		{"toSnakeCase", "http_server_id"},
		{"toKebabCase", "http-server-id"},
		{"toConstantCase", "HTTP_SERVER_ID"},
		{"toPascalCase", "HttpServerId"},
		{"toCamelCase", "httpServerId"},
		{"toGoPascalCase", "HTTPServerID"},
		{"toGoCamelCase", "httpServerID"},
	}
	for _, tt := range tests {
		if got := funcs[tt.fn].(func(string) string)(in); got != tt.want {
			t.Errorf("%s(%q) = %q, want %q", tt.fn, in, got, tt.want)
		}
	}
}

func TestFuncs(t *testing.T) {
	type item struct{ Name, Dir string }
	type ranked struct {
		Name   string
		Rank   int
		Size   uint
		Weight float64
		Flag   bool
	}
	data := map[string]any{
		"Items":    []item{{"b", "x"}, {"a", "y"}, {"c", "x"}},
		"Names":    []string{"a", "b"},
		"Mixed":    []any{1, "two", true},
		"Doc":      "line one\nline two",
		"Empty":    []item{},
		"Ranked":   []ranked{{Name: "x", Rank: 3, Size: 2, Weight: 0.5}, {Name: "y", Rank: 1, Size: 3, Weight: 1.5}, {Name: "z", Rank: 2, Size: 1, Weight: 2.5}},
		"Pointers": []*ranked{{Name: "x", Rank: 2}, {Name: "y", Rank: 1}},
		"Mixed2":   []any{struct{ Key string }{"a"}, struct{ Key int }{1}},
	}
	tests := []struct {
		name    string
		text    string
		want    string
		wantErr bool
	}{
		{name: "join strings", text: `{{ join ", " .Names }}`, want: "a, b"},
		{name: "join any values", text: `{{ join "|" .Mixed }}`, want: "1|two|true"},
		{name: "join in a pipeline", text: `{{ fieldValues "Name" .Items | join "," }}`, want: "b,a,c"},
		{name: "join an empty list", text: `{{ join "," (fieldValues "Name" .Empty) }}`, want: ""},
		{name: "join not a list", text: `{{ join "," "abc" }}`, wantErr: true},
		{name: "trimPrefix", text: `{{ trimPrefix "Handle" "HandleUsers" }}`, want: "Users"},
		{name: "trimPrefix without the prefix", text: `{{ "Users" | trimPrefix "Handle" }}`, want: "Users"},
		{name: "trimSuffix", text: `{{ trimSuffix "Handler" "UsersHandler" }}`, want: "Users"},
		{name: "replace", text: `{{ replace "-" "_" "go-redis-v2" }}`, want: "go_redis_v2"},
		{name: "quote", text: `{{ quote "s3 \"fast\"" }}`, want: `"s3 \"fast\""`},
		{name: "quote a number", text: `{{ quote 42 }}`, want: `"42"`},
		{name: "quote nothing", text: `{{ quote nil }}`, want: ""},
		{name: "quote takes one value", text: `{{ quote "a" "b" }}`, wantErr: true},
		{name: "indent", text: `{{ indent 2 .Doc }}`, want: "  line one\n  line two"},
		{name: "nindent", text: `x:{{ nindent 4 .Doc }}`, want: "x:\n    line one\n    line two"},
		{name: "indent by zero", text: `{{ indent 0 "a" }}`, want: "a"},
		{name: "indent by a negative number", text: `{{ indent -1 "a" }}`, wantErr: true},
		{name: "nindent by a negative number", text: `{{ nindent -1 "a" }}`, wantErr: true},
		{name: "plural of none", text: `{{ plural "driver" "drivers" 0 }}`, want: "drivers"},
		{name: "plural of one", text: `{{ plural "driver" "drivers" 1 }}`, want: "driver"},
		{name: "plural of many", text: `{{ len .Items | plural "driver" "drivers" }}`, want: "drivers"},
		{name: "plural built from a name", text: `{{ $n := "Driver" }}{{ plural $n (print $n "s") 2 }}`, want: "Drivers"},
		{
			name: "groupBy sorts groups and keeps item order",
			text: `{{ range groupBy "Dir" .Items }}{{ .Key }}={{ range .Items }}{{ .Name }}{{ end }} {{ end }}`,
			want: "x=bc y=a ",
		},
		{name: "groupBy an empty list", text: `{{ len (groupBy "Dir" .Empty) }}`, want: "0"},
		{name: "sortBy a string field", text: `{{ range sortBy "Name" .Items }}{{ .Name }}{{ end }}`, want: "abc"},
		{name: "sortBy keeps the order of equal values", text: `{{ range sortBy "Dir" .Items }}{{ .Name }}{{ end }}`, want: "bca"},
		{name: "sortBy a number", text: `{{ range sortBy "Rank" .Ranked }}{{ .Name }}{{ end }}`, want: "yzx"},
		{name: "sortBy an unsigned number", text: `{{ range sortBy "Size" .Ranked }}{{ .Name }}{{ end }}`, want: "zxy"},
		{name: "sortBy a float", text: `{{ range sortBy "Weight" .Ranked }}{{ .Name }}{{ end }}`, want: "xyz"},
		{name: "sortBy pointers to structs", text: `{{ range sortBy "Rank" .Pointers }}{{ .Name }}{{ end }}`, want: "yx"},
		{name: "sortBy keeps the list's type", text: `{{ sortBy "Name" .Items | fieldValues "Dir" | join "," }}`, want: "y,x,x"},
		{name: "sortBy leaves the list alone", text: `{{ $s := sortBy "Name" .Items }}{{ range .Items }}{{ .Name }}{{ end }}`, want: "bac"},
		{name: "sortBy an empty list", text: `{{ len (sortBy "Name" .Empty) }}`, want: "0"},
		{name: "sortBy a field of another kind", text: `{{ sortBy "Flag" .Ranked }}`, wantErr: true},
		{name: "sortBy a missing field", text: `{{ sortBy "Nope" .Items }}`, wantErr: true},
		{name: "sortBy not a list", text: `{{ sortBy "Name" "abc" }}`, wantErr: true},
		{name: "sortBy a list of strings", text: `{{ sortBy "Name" .Names }}`, wantErr: true},
		{name: "sortBy fields of different kinds", text: `{{ sortBy "Key" .Mixed2 }}`, wantErr: true},
		{name: "groupBy a missing field", text: `{{ groupBy "Nope" .Items }}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderText(tt.text, data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Text(%s) error = %v, wantErr %v", tt.text, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Text(%s) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestGroupByEmpty(t *testing.T) {
	groups, err := groupBy("Name", []struct{ Name string }{})
	if err != nil || groups == nil || len(groups) != 0 {
		t.Errorf("groupBy() = %#v, %v, want an empty list", groups, err)
	}
}

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
	data := map[string]any{
		"Items": []item{{"b", "x"}, {"a", "y"}, {"c", "x"}},
		"Names": []string{"a", "b"},
		"Mixed": []any{1, "two", true},
		"Doc":   "line one\nline two",
		"Empty": []item{},
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
		{name: "groupBy a missing field", text: `{{ groupBy "Nope" .Items }}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Text(tt.text, data)
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

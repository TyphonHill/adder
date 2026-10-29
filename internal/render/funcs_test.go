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

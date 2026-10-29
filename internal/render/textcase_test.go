package render

import (
	"slices"
	"testing"
)

func TestWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"user", []string{"user"}},
		{"userAccount", []string{"user", "Account"}},
		{"UserAccount", []string{"User", "Account"}},
		{"user_account", []string{"user", "account"}},
		{"user-account id", []string{"user", "account", "id"}},
		{"  __user..account__  ", []string{"user", "account"}},
		{"userID", []string{"user", "ID"}},
		{"HTTPServer", []string{"HTTP", "Server"}},
		{"HTTP2Server", []string{"HTTP2", "Server"}},
		{"ServeHTTP", []string{"Serve", "HTTP"}},
		{"ID3Tag", []string{"ID3", "Tag"}},
		{"version2update", []string{"version2update"}},
		{"Version2Update", []string{"Version2", "Update"}},
		{"ChildBMCHealthCheckWorkflow", []string{"Child", "BMC", "Health", "Check", "Workflow"}},
		// Mixed-case acronyms are ambiguous; write ipv4_address to get the intended split.
		{"IPv4Address", []string{"I", "Pv4", "Address"}},
		{"ÉcoleNormale", []string{"École", "Normale"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := words(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("words(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCaseFuncs(t *testing.T) {
	tests := []struct {
		name, in                              string
		lower, upper                          string
		snake, kebab, constant, pascal, camel string
		goPascal, goCamel                     string
	}{
		{"snake input", "user_account", "user_account", "USER_ACCOUNT", "user_account", "user-account", "USER_ACCOUNT", "UserAccount", "userAccount", "UserAccount", "userAccount"},
		{"initialism", "user_id", "user_id", "USER_ID", "user_id", "user-id", "USER_ID", "UserId", "userId", "UserID", "userID"},
		{"leading acronym", "HTTPServer", "httpserver", "HTTPSERVER", "http_server", "http-server", "HTTP_SERVER", "HttpServer", "httpServer", "HTTPServer", "httpServer"},
		{"kebab input", "url-path", "url-path", "URL-PATH", "url_path", "url-path", "URL_PATH", "UrlPath", "urlPath", "URLPath", "urlPath"},
		{"trailing acronym", "ServeHTTP", "servehttp", "SERVEHTTP", "serve_http", "serve-http", "SERVE_HTTP", "ServeHttp", "serveHttp", "ServeHTTP", "serveHTTP"},
		{"acronym that is not an initialism", "ChildBMCHealthCheckWorkflow", "childbmchealthcheckworkflow", "CHILDBMCHEALTHCHECKWORKFLOW", "child_bmc_health_check_workflow", "child-bmc-health-check-workflow", "CHILD_BMC_HEALTH_CHECK_WORKFLOW", "ChildBmcHealthCheckWorkflow", "childBmcHealthCheckWorkflow", "ChildBMCHealthCheckWorkflow", "childBMCHealthCheckWorkflow"},
		{"leading acronym that is not an initialism", "BMCHealth", "bmchealth", "BMCHEALTH", "bmc_health", "bmc-health", "BMC_HEALTH", "BmcHealth", "bmcHealth", "BMCHealth", "bmcHealth"},
		{"single letter word", "get_a_host", "get_a_host", "GET_A_HOST", "get_a_host", "get-a-host", "GET_A_HOST", "GetAHost", "getAHost", "GetAHost", "getAHost"},
		{"empty", "", "", "", "", "", "", "", "", "", ""},
	}
	funcs := Funcs()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range []struct{ fn, want string }{
				{"toLower", tt.lower},
				{"toUpper", tt.upper},
				{"toSnakeCase", tt.snake},
				{"toKebabCase", tt.kebab},
				{"toConstantCase", tt.constant},
				{"toPascalCase", tt.pascal},
				{"toCamelCase", tt.camel},
				{"toGoPascalCase", tt.goPascal},
				{"toGoCamelCase", tt.goCamel},
			} {
				t.Run(c.fn, func(t *testing.T) {
					if got := funcs[c.fn].(func(string) string)(tt.in); got != c.want {
						t.Errorf("%s(%q) = %q, want %q", c.fn, tt.in, got, c.want)
					}
				})
			}
		})
	}
}

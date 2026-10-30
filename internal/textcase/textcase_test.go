package textcase

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

func TestCase(t *testing.T) {
	tests := []struct {
		name, in                              string
		snake, kebab, constant, pascal, camel string
		goPascal, goCamel                     string
	}{
		{"snake input", "user_account", "user_account", "user-account", "USER_ACCOUNT", "UserAccount", "userAccount", "UserAccount", "userAccount"},
		{"initialism", "user_id", "user_id", "user-id", "USER_ID", "UserId", "userId", "UserID", "userID"},
		{"leading acronym", "HTTPServer", "http_server", "http-server", "HTTP_SERVER", "HttpServer", "httpServer", "HTTPServer", "httpServer"},
		{"kebab input", "url-path", "url_path", "url-path", "URL_PATH", "UrlPath", "urlPath", "URLPath", "urlPath"},
		{"trailing acronym", "ServeHTTP", "serve_http", "serve-http", "SERVE_HTTP", "ServeHttp", "serveHttp", "ServeHTTP", "serveHTTP"},
		{"acronym that is not an initialism", "ChildBMCHealthCheckWorkflow", "child_bmc_health_check_workflow", "child-bmc-health-check-workflow", "CHILD_BMC_HEALTH_CHECK_WORKFLOW", "ChildBmcHealthCheckWorkflow", "childBmcHealthCheckWorkflow", "ChildBMCHealthCheckWorkflow", "childBMCHealthCheckWorkflow"},
		{"leading acronym that is not an initialism", "BMCHealth", "bmc_health", "bmc-health", "BMC_HEALTH", "BmcHealth", "bmcHealth", "BMCHealth", "bmcHealth"},
		{"initialism staticcheck adds", "user_db", "user_db", "user-db", "USER_DB", "UserDb", "userDb", "UserDB", "userDB"},
		{"initialism only golint had", "lhs_value", "lhs_value", "lhs-value", "LHS_VALUE", "LhsValue", "lhsValue", "LhsValue", "lhsValue"},
		{"capitals in the input win", "user_SDK", "user_sdk", "user-sdk", "USER_SDK", "UserSdk", "userSdk", "UserSDK", "userSDK"},
		{"single letter word", "get_a_host", "get_a_host", "get-a-host", "GET_A_HOST", "GetAHost", "getAHost", "GetAHost", "getAHost"},
		{"empty", "", "", "", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, c := range []struct {
				name string
				fn   func(string) string
				want string
			}{
				{"Snake", Snake, tt.snake},
				{"Kebab", Kebab, tt.kebab},
				{"Constant", Constant, tt.constant},
				{"Pascal", Pascal, tt.pascal},
				{"Camel", Camel, tt.camel},
				{"GoPascal", GoPascal, tt.goPascal},
				{"GoCamel", GoCamel, tt.goCamel},
			} {
				t.Run(c.name, func(t *testing.T) {
					if got := c.fn(tt.in); got != c.want {
						t.Errorf("%s(%q) = %q, want %q", c.name, tt.in, got, c.want)
					}
				})
			}
		})
	}
}

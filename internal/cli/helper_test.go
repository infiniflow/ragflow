package cli

import "testing"

func TestAPIModeClient(t *testing.T) {
	web := &HTTPClient{LoginToken: new("token")}
	api := &HTTPClient{useAPIKey: true}
	config := &CommandLineConfig{CLIMode: APIMode, APIClientConfig: &APIModeConfig{CurrentAPIServer: "test"}}
	for _, test := range []struct {
		name   string
		config *CommandLineConfig
		client *HTTPClient
		want   *HTTPClient
	}{
		{name: "missing config"},
		{name: "wrong mode", config: &CommandLineConfig{}},
		{name: "missing API config", config: &CommandLineConfig{CLIMode: APIMode}},
		{name: "missing server", config: config},
		{name: "missing authorization", config: config, client: &HTTPClient{}},
		{name: "web token", config: config, client: web, want: web},
		{name: "API key", config: config, client: api, want: api},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := &CLI{Config: test.config, APIServerClientMap: map[string]*HTTPClient{"test": test.client}}
			got, err := c.apiModeClient()
			if got != test.want || (err != nil) != (test.want == nil) {
				t.Fatalf("apiModeClient() = %p, %v; want %p", got, err, test.want)
			}
		})
	}
}

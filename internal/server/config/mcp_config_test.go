package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func clearMCPEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ENABLED", "HOST", "PORT", "LAUNCH_MODE", "HOST_API_KEY", "TRANSPORT_SSE_ENABLED", "TRANSPORT_STREAMABLE_ENABLED", "JSON_RESPONSE"} {
		name := "RAGFLOW_MCP_" + key
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func parseMCPYAML(t *testing.T, text string) (MCPConfig, error) {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := v.ReadConfig(strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	err := cfg.ParseAPIServerConfig(v)
	return cfg.GetAPIServerConfig().MCP, err
}

func TestMCPConfiguration(t *testing.T) {
	clearMCPEnv(t)
	defaults := MCPConfig{Host: "127.0.0.1", Port: 9382, LaunchMode: "self-host", SSE: true, StreamableHTTP: true, JSONResponse: true}
	got, err := parseMCPYAML(t, "{}")
	if err != nil || got != defaults {
		t.Fatalf("defaults: %+v, %v", got, err)
	}
	yaml := `mcp:
  enabled: true
  host: yaml-host
  port: 9999
  launch_mode: host
  host_api_key: yaml-key
  transport_sse_enabled: false
  transport_streamable_enabled: true
  json_response: false
`
	got, err = parseMCPYAML(t, yaml)
	want := MCPConfig{Enabled: true, Host: "yaml-host", Port: 9999, LaunchMode: "host", HostAPIKey: "yaml-key", StreamableHTTP: true}
	if err != nil || got != want {
		t.Fatalf("YAML: %+v, %v", got, err)
	}
	overrides := map[string]string{"ENABLED": "yes", "HOST": "env-host", "PORT": "10002", "LAUNCH_MODE": "self-host", "HOST_API_KEY": "env-key", "TRANSPORT_SSE_ENABLED": "ON", "TRANSPORT_STREAMABLE_ENABLED": "1", "JSON_RESPONSE": " True "}
	for k, v := range overrides {
		t.Setenv("RAGFLOW_MCP_"+k, v)
	}
	got, err = parseMCPYAML(t, yaml)
	want = MCPConfig{Enabled: true, Host: "env-host", Port: 10002, LaunchMode: "self-host", HostAPIKey: "env-key", SSE: true, StreamableHTTP: true, JSONResponse: true}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("environment precedence: %+v, %v", got, err)
	}
	t.Setenv("RAGFLOW_MCP_ENABLED", "")
	t.Setenv("RAGFLOW_MCP_HOST", "")
	t.Setenv("RAGFLOW_MCP_HOST_API_KEY", "")
	got, err = parseMCPYAML(t, yaml)
	if err != nil || got.Enabled || got.Host != "" || got.HostAPIKey != "" {
		t.Fatalf("empty overrides ignored: %+v, %v", got, err)
	}
}

func TestMCPTransportFallback(t *testing.T) {
	clearMCPEnv(t)
	for _, sse := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, json := range []bool{false, true} {
				v := viper.New()
				v.Set("mcp.transport_sse_enabled", sse)
				v.Set("mcp.transport_streamable_enabled", stream)
				v.Set("mcp.json_response", json)
				var cfg Config
				if err := cfg.ParseAPIServerConfig(v); err != nil {
					t.Fatal(err)
				}
				got := cfg.GetAPIServerConfig().MCP
				if got.SSE != sse || got.StreamableHTTP != (stream || !sse) || got.JSONResponse != (json && stream) {
					t.Fatalf("%v/%v/%v -> %+v", sse, stream, json, got)
				}
			}
		}
	}
}

func TestMCPValidation(t *testing.T) {
	for _, tc := range []struct{ name, key, value, yaml string }{
		{"zero port", "PORT", "0", "{}"}, {"negative port", "PORT", "-1", "{}"}, {"large port", "PORT", "65536", "{}"}, {"bad port", "PORT", "bad", "{}"}, {"empty port", "PORT", "", "{}"},
		{"bad mode", "LAUNCH_MODE", "bogus", "{}"}, {"empty mode", "LAUNCH_MODE", "", "{}"}, {"enabled key missing", "ENABLED", "true", "{}"},
		{"empty key overrides YAML", "HOST_API_KEY", "", "mcp: {enabled: true, host_api_key: file-key}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearMCPEnv(t)
			t.Setenv("RAGFLOW_MCP_"+tc.key, tc.value)
			if _, err := parseMCPYAML(t, tc.yaml); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, yaml := range []string{"mcp: {port: 0}", "mcp: {port: 3.14}", "mcp: {launch_mode: bogus}", "mcp: {enabled: true}"} {
		t.Run(yaml, func(t *testing.T) {
			clearMCPEnv(t)
			if _, err := parseMCPYAML(t, yaml); err == nil {
				t.Fatal("invalid YAML configuration accepted")
			}
		})
	}
	t.Run("host mode needs no fixed key", func(t *testing.T) {
		clearMCPEnv(t)
		if _, err := parseMCPYAML(t, "mcp: {enabled: true, launch_mode: host}"); err != nil {
			t.Fatal(err)
		}
	})
	for _, value := range []string{"false", "0", "no", "off", "invalid", ""} {
		t.Run("false-"+value, func(t *testing.T) {
			clearMCPEnv(t)
			t.Setenv("RAGFLOW_MCP_ENABLED", value)
			got, err := parseMCPYAML(t, "{}")
			if err != nil || got.Enabled {
				t.Fatalf("existing false spelling changed: %+v, %v", got, err)
			}
		})
	}
}

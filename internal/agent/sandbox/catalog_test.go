package sandbox

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"testing"
)

func TestCatalogOrderAndFreshSchemas(t *testing.T) {
	providers := ListProviders()
	if len(providers) != 7 {
		t.Fatalf("provider count = %d, want 7", len(providers))
	}
	for i, want := range providerOrder {
		if got := providers[i]["id"]; got != want {
			t.Errorf("provider %d = %v, want %q", i, got, want)
		}
	}
	a, err := ConfigSchema("local")
	if err != nil {
		t.Fatal(err)
	}
	a["python_bin"] = nil
	b, err := ConfigSchema("local")
	if err != nil {
		t.Fatal(err)
	}
	if b["python_bin"] == nil {
		t.Fatal("ConfigSchema returned shared mutable state")
	}
}

func TestCatalogValidationBoundaries(t *testing.T) {
	baselines := map[string]map[string]any{
		"local":                  {},
		"self_managed":           {"endpoint": "http://localhost"},
		"ssh":                    {"host": "host", "username": "user", "port": 22, "password": "password", "timeout": 30, "max_output_bytes": 1024, "max_artifact_bytes": 1024},
		"aliyun_codeinterpreter": {"access_key_id": "LTAItest", "access_key_secret": "secret", "account_id": "account"},
		"e2b":                    {"api_key": "key"},
		"tenki":                  {"api_key": "key", "timeout": 30, "max_lifetime": 3600, "max_output_bytes": 1024, "max_artifact_bytes": 1024},
		"ucloud_agent_sandbox":   {"api_key": "key"},
	}
	for provider, base := range baselines {
		t.Run(provider, func(t *testing.T) {
			if err := ValidateConfig(provider, base); err != nil {
				t.Fatalf("valid baseline rejected: %v", err)
			}
			if err := ValidateConfig(provider, nil); err == nil {
				t.Fatal("null object accepted")
			}
			schema, err := ConfigSchema(provider)
			if err != nil {
				t.Fatal(err)
			}
			for name, raw := range schema {
				field := raw.(map[string]any)
				if field["required"] == true {
					cfg := maps.Clone(base)
					delete(cfg, name)
					if err := ValidateConfig(provider, cfg); err == nil {
						t.Errorf("missing required field %s accepted", name)
					}
				}
				wrong := []any{nil, []any{}, map[string]any{}}
				switch field["type"] {
				case "string":
					wrong = append(wrong, 1, true)
				case "boolean":
					wrong = append(wrong, "false", 0)
				case "integer":
					wrong = append(wrong, true, "1", 1.5, math.Inf(1), math.NaN())
				}
				for _, value := range wrong {
					cfg := maps.Clone(base)
					cfg[name] = value
					if err := ValidateConfig(provider, cfg); err == nil {
						t.Errorf("%s accepted wrong type/value %T %v", name, value, value)
					}
				}
				if field["type"] == "integer" {
					lo, hi := integer(field["min"]), integer(field["max"])
					for _, value := range []int64{lo - 1, lo, hi, hi + 1} {
						cfg := maps.Clone(base)
						cfg[name] = float64(value)
						err := ValidateConfig(provider, cfg)
						if (err != nil) != (value < lo || value > hi) {
							t.Errorf("%s=%d error=%v", name, value, err)
						}
					}
				}
			}
			cfg := maps.Clone(base)
			cfg["unknown_setting"] = map[string]any{"nested": []any{1, "value"}}
			if err := ValidateConfig(provider, cfg); err != nil {
				t.Fatalf("unknown key rejected: %v", err)
			}
		})
	}
	if _, err := ConfigSchema("unknown"); err == nil {
		t.Fatal("unknown schema accepted")
	}
	if err := ValidateConfig("unknown", map[string]any{}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestCatalogDynamicDeploymentDefaults(t *testing.T) {
	for env, field := range map[string]string{"SANDBOX_EXECUTOR_MANAGER_IMAGE": "executor_manager_image", "SANDBOX_BASE_PYTHON_IMAGE": "base_python_image", "SANDBOX_BASE_NODEJS_IMAGE": "base_nodejs_image", "SANDBOX_MAX_MEMORY": "max_memory", "SANDBOX_CONTAINER_NETWORK": "container_network", "SANDBOX_TIMEOUT": "sandbox_timeout"} {
		t.Setenv(env, "custom")
		schema, err := ConfigSchema("self_managed")
		if err != nil {
			t.Fatal(err)
		}
		if schema[field].(map[string]any)["default"] != "custom" {
			t.Errorf("%s default not refreshed", field)
		}
		t.Setenv(env, "")
		schema, err = ConfigSchema("self_managed")
		if err != nil {
			t.Fatal(err)
		}
		if schema[field].(map[string]any)["default"] != "" {
			t.Errorf("%s empty env lost", field)
		}
	}
	t.Setenv("SANDBOX_ENABLE_SECCOMP", "TRUE")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE", "9")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_PORT", "9999")
	schema, err := ConfigSchema("self_managed")
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]any{"enable_seccomp": true, "executor_manager_pool_size": int64(9), "executor_manager_port": int64(9999)} {
		if got := schema[field].(map[string]any)["default"]; got != want {
			t.Errorf("%s=%v want %v", field, got, want)
		}
	}
	providers := ListProviders()
	providers[0]["tags"].([]any)[0] = "changed"
	if fmt.Sprint(ListProviders()[0]["tags"]) == fmt.Sprint(providers[0]["tags"]) {
		t.Fatal("provider tags share mutable state")
	}
	schema["endpoint"].(map[string]any)["default"] = "changed"
	fresh, err := ConfigSchema("self_managed")
	if err != nil {
		t.Fatal(err)
	}
	if fresh["endpoint"].(map[string]any)["default"] == "changed" {
		t.Fatal("nested schema shares mutable state")
	}
}

func TestValidateConfigTypesRangesAndProviderRules(t *testing.T) {
	if err := ValidateConfig("local", map[string]any{"timeout": true}); err == nil {
		t.Fatal("boolean accepted as integer")
	} else {
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error type = %T, want ValidationError", err)
		}
	}
	if err := ValidateConfig("local", map[string]any{"timeout": 1.5}); err == nil {
		t.Fatal("fractional integer accepted")
	}
	if err := ValidateConfig("ssh", map[string]any{"host": "h", "username": "u"}); err == nil {
		t.Fatal("SSH without authentication accepted")
	}
	if err := ValidateConfig("aliyun_codeinterpreter", map[string]any{
		"access_key_id": "bad", "access_key_secret": "secret", "account_id": "account",
	}); err == nil {
		t.Fatal("invalid Aliyun access key accepted")
	}
	if err := ValidateConfig("tenki", map[string]any{"api_key": "key", "timeout": 0}); err == nil {
		t.Fatal("invalid Tenki timeout accepted")
	}
	if err := ValidateConfig("ucloud_agent_sandbox", map[string]any{"api_key": "key", "template": ""}); err == nil {
		t.Fatal("empty UCloud template accepted")
	}
}

func TestSelfManagedPoolValidationUsesSelectedField(t *testing.T) {
	cfg := map[string]any{"endpoint": "http://localhost", "executor_manager_pool_size": 3, "pool_size": -1}
	if err := ValidateConfig("self_managed", cfg); err != nil {
		t.Fatal(err)
	}
	delete(cfg, "executor_manager_pool_size")
	if err := ValidateConfig("self_managed", cfg); err == nil {
		t.Fatal("invalid fallback pool accepted")
	}
}

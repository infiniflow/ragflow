//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import "testing"

func TestMinerUProviderConfigFromAPIKey(t *testing.T) {
	t.Run("plain bearer token", func(t *testing.T) {
		cfg := MinerUProviderConfigFromAPIKey("secret-token")
		if cfg.IsProviderJSON || cfg.AccessToken != "secret-token" {
			t.Fatalf("cfg = %#v, want plain token", cfg)
		}
	})

	t.Run("provider JSON without bearer", func(t *testing.T) {
		raw := `{"mineru_apiserver":"http://mineru:9987","mineru_backend":"vlm-http-client","mineru_server_url":"http://vllm:30000"}`
		cfg := MinerUProviderConfigFromAPIKey(raw)
		if !cfg.IsProviderJSON {
			t.Fatal("expected provider JSON")
		}
		if cfg.AccessToken != "" {
			t.Fatalf("AccessToken = %q, want empty", cfg.AccessToken)
		}
		if cfg.APIServer != "http://mineru:9987" || cfg.Backend != "vlm-http-client" || cfg.ServerURL != "http://vllm:30000" {
			t.Fatalf("cfg = %#v", cfg)
		}
	})

	t.Run("provider JSON with explicit token", func(t *testing.T) {
		raw := `{"mineru_apiserver":"http://mineru:9987","mineru_api_key":"bearer-secret"}`
		cfg := MinerUProviderConfigFromAPIKey(raw)
		if cfg.AccessToken != "bearer-secret" {
			t.Fatalf("AccessToken = %q", cfg.AccessToken)
		}
	})

	t.Run("access_token only JSON object", func(t *testing.T) {
		raw := `{"access_token":"secret"}`
		cfg := MinerUProviderConfigFromAPIKey(raw)
		if !cfg.IsProviderJSON {
			t.Fatal("expected provider JSON")
		}
		if cfg.AccessToken != "secret" {
			t.Fatalf("AccessToken = %q, want secret", cfg.AccessToken)
		}
	})

	t.Run("JSON object without token fields", func(t *testing.T) {
		raw := `{"other_field":"value"}`
		cfg := MinerUProviderConfigFromAPIKey(raw)
		if !cfg.IsProviderJSON {
			t.Fatal("expected provider JSON")
		}
		if cfg.AccessToken != "" {
			t.Fatalf("AccessToken = %q, want empty", cfg.AccessToken)
		}
	})
}

func TestResolveMinerUBackendAndServerURL(t *testing.T) {
	apiKey := `{"mineru_backend":"hybrid-http-client","mineru_server_url":"http://vllm:30000"}`
	if got := ResolveMinerUBackend("", apiKey); got != "hybrid-http-client" {
		t.Fatalf("backend = %q", got)
	}
	if got := ResolveMinerUServerURL("", apiKey); got != "http://vllm:30000" {
		t.Fatalf("server_url = %q", got)
	}
	if got := ResolveMinerUBackend("pipeline", apiKey); got != "pipeline" {
		t.Fatalf("setup should win, backend = %q", got)
	}
}

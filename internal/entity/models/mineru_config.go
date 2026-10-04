//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"fmt"
	"ragflow/internal/common"
	"strings"
)

var ValidMinerUBackends = map[string]struct{}{
	"pipeline":           {},
	"vlm-engine":         {},
	"hybrid-engine":      {},
	"vlm-http-client":    {},
	"hybrid-http-client": {},
}

func MinerUBackendRequiresServerURL(backend string) bool {
	return backend == "vlm-http-client" || backend == "hybrid-http-client"
}

func ValidateMinerUConfig(backend, serverURL string) error {
	if _, ok := ValidMinerUBackends[backend]; !ok {
		return fmt.Errorf(
			"parser: MinerU invalid backend %q (valid: pipeline, vlm-engine, hybrid-engine, vlm-http-client, hybrid-http-client)",
			backend,
		)
	}
	if MinerUBackendRequiresServerURL(backend) && serverURL == "" {
		return fmt.Errorf("parser: MinerU requires mineru_server_url or MINERU_SERVER_URL for backend %q", backend)
	}
	return nil
}

// MinerUProviderAPIKeyConfig holds fields stored in the tenant MinerU provider api_key JSON.
type MinerUProviderAPIKeyConfig struct {
	IsProviderJSON bool
	APIServer      string
	Backend        string
	ServerURL      string
	AccessToken    string
}

// MinerUProviderConfigFromAPIKey parses the tenant MinerU api_key payload the same way
// the provider UI stores it: mineru_apiserver, mineru_backend, mineru_server_url, etc.
// A non-JSON api_key is treated as a plain bearer token. JSON provider config does not
// imply a bearer token unless mineru_api_key or access_token is present.
func MinerUProviderConfigFromAPIKey(apiKey string) MinerUProviderAPIKeyConfig {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return MinerUProviderAPIKeyConfig{}
	}
	raw := providerJSONConfigMap(trimmed)
	if raw == nil {
		return MinerUProviderAPIKeyConfig{AccessToken: trimmed}
	}
	get := func(key string) string {
		if value, ok := raw[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		return ""
	}
	token := get("mineru_api_key")
	if token == "" {
		token = get("access_token")
	}
	return MinerUProviderAPIKeyConfig{
		IsProviderJSON: true,
		APIServer:      get("mineru_apiserver"),
		Backend:        get("mineru_backend"),
		ServerURL:      strings.TrimRight(get("mineru_server_url"), "/"),
		AccessToken:    token,
	}
}

// MinerUBearerTokenFromAPIKey returns the Authorization bearer secret for MinerU HTTP calls.
func MinerUBearerTokenFromAPIKey(apiKey string) string {
	return MinerUProviderConfigFromAPIKey(apiKey).AccessToken
}

func ResolveMinerUBackend(setupBackend, apiKey string) string {
	backend := strings.TrimSpace(setupBackend)
	if backend == "" {
		cfg := MinerUProviderConfigFromAPIKey(apiKey)
		if cfg.Backend != "" {
			backend = cfg.Backend
		}
	}
	if backend == "" {
		backend = strings.TrimSpace(common.GetEnv(common.EnvMineruBackend))
	}
	if backend == "" {
		backend = "pipeline"
	}
	return backend
}

func ResolveMinerUServerURL(setupServerURL, apiKey string) string {
	serverURL := strings.TrimSpace(setupServerURL)
	if serverURL == "" {
		cfg := MinerUProviderConfigFromAPIKey(apiKey)
		if cfg.ServerURL != "" {
			serverURL = cfg.ServerURL
		}
	}
	if serverURL == "" {
		serverURL = strings.TrimSpace(common.GetEnv(common.EnvMineruServerURL))
	}
	return strings.TrimRight(serverURL, "/")
}

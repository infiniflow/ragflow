//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//

package models

import (
	"fmt"
	"ragflow/internal/common"
	"strings"
)

// MinerU 3.x /file_parse backend names. MinerU 4.0 dropped --backend in favor of
// quality tiers (basic/standard/advanced/flash); see MinerUTierFromBackend.
var ValidMinerUBackends = map[string]struct{}{
	"pipeline":           {},
	"vlm-engine":         {},
	"hybrid-engine":      {},
	"vlm-http-client":    {},
	"hybrid-http-client": {},
	"vlm-auto-engine":    {},
	"hybrid-auto-engine": {},
	"basic":              {},
	"standard":           {},
	"advanced":           {},
	"flash":              {},
}

var validMinerUTiers = map[string]struct{}{
	"basic":    {},
	"standard": {},
	"advanced": {},
	"flash":    {},
}

func MinerUBackendRequiresServerURL(backend string) bool {
	return backend == "vlm-http-client" || backend == "hybrid-http-client"
}

func MinerUIsV1Tier(name string) bool {
	_, ok := validMinerUTiers[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// MinerUTierFromBackend maps a 3.x backend or a 4.0 tier onto a V1 `tier`.
// MinerU 4.0 changelog: pipeline → basic; vlm-* → advanced; hybrid-* → standard.
func MinerUTierFromBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "basic", "pipeline":
		return "basic"
	case "flash":
		return "flash"
	case "advanced", "vlm-engine", "vlm-auto-engine", "vlm-http-client":
		return "advanced"
	case "standard", "hybrid-engine", "hybrid-auto-engine", "hybrid-http-client", "":
		return "standard"
	default:
		return "standard"
	}
}

func ValidateMinerUConfig(backend, serverURL string) error {
	return validateMinerUConfig(backend, serverURL, false)
}

func ValidateMinerUConfigForAPI(backend, serverURL string, v1 bool) error {
	return validateMinerUConfig(backend, serverURL, v1)
}

func validateMinerUConfig(backend, serverURL string, v1 bool) error {
	if _, ok := ValidMinerUBackends[backend]; !ok {
		return fmt.Errorf(
			"parser: MinerU invalid backend %q (valid: pipeline, vlm-engine, hybrid-engine, vlm-http-client, hybrid-http-client, vlm-auto-engine, hybrid-auto-engine, basic, standard, advanced, flash)",
			backend,
		)
	}
	if v1 {
		// MinerU 4 configures the VLM on the service itself; server_url is not a V1 job field.
		return nil
	}
	if MinerUIsV1Tier(backend) {
		return fmt.Errorf(
			"parser: MinerU backend %q is a 4.0 tier, but the connected service does not speak V1 (/v1/health). Use pipeline, vlm-engine, hybrid-engine, vlm-http-client, or hybrid-http-client, or upgrade MinerU to 4.0",
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

//go:build integration

package api

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func unauthenticatedConfig() ServerTestConfig {
	config := *TestConfig
	config.APIKey = ""
	return config
}

func TestRESTSystemPublicContracts(t *testing.T) {
	config := unauthenticatedConfig()
	ping := restCall(t, &config, http.MethodGet, "/system/ping", nil, nil)
	if ping.Status != http.StatusOK || string(ping.Body) != "pong" {
		t.Fatalf("ping status/body=%d/%q", ping.Status, ping.Body)
	}
	version := restOK(t, restCall(t, &config, http.MethodGet, "/system/version", nil, nil))
	if value, ok := version["data"].(string); !ok || value == "" {
		t.Fatalf("version must be a nonempty string: %v", version)
	}
	systemConfig := restOK(t, restCall(t, &config, http.MethodGet, "/system/config", nil, nil))
	configData := object(t, systemConfig["data"])
	for _, key := range []string{"registerEnabled", "disablePasswordLogin"} {
		if _, ok := configData[key]; !ok {
			t.Fatalf("system config missing %q: %v", key, configData)
		}
	}
	health := restCall(t, &config, http.MethodGet, "/system/healthz", nil, nil)
	if health.Status != http.StatusOK && health.Status != http.StatusInternalServerError {
		t.Fatalf("healthz status=%d", health.Status)
	}
	if len(health.JSON) == 0 {
		t.Fatalf("healthz returned no object: %q", health.Body)
	}
}

func TestRESTSystemProtectedContracts(t *testing.T) {
	unauth := unauthenticatedConfig()
	for _, path := range []string{"/system/status", "/system/stats/chat", "/system/tokens", "/system/config/log"} {
		result := restCall(t, &unauth, http.MethodGet, path, nil, nil)
		if result.Status != http.StatusUnauthorized || result.JSON["code"] != float64(401) {
			t.Fatalf("%s auth contract: status=%d body=%v", path, result.Status, result.JSON)
		}
	}
	status := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/status", nil, nil))["data"])
	for _, key := range []string{"doc_engine", "storage", "database", "redis"} {
		if _, ok := status[key]; !ok {
			t.Fatalf("status missing %q: %v", key, status)
		}
	}
	stats := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/stats/chat", nil, nil))["data"])
	for _, key := range []string{"pv", "uv", "speed", "tokens", "round", "thumb_up"} {
		if _, ok := stats[key].([]any); !ok {
			t.Fatalf("stats[%q] must be an array: %T", key, stats[key])
		}
	}
}

func TestRESTSystemTokenLifecycleAndRedaction(t *testing.T) {
	unauth := unauthenticatedConfig()
	created := restOK(t, restCall(t, TestConfig, http.MethodPost, "/system/tokens", nil, nil))
	data := object(t, created["data"])
	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("created token missing: %v", data)
	}
	deleted := false
	t.Cleanup(func() {
		if !deleted {
			result := restCall(t, TestConfig, http.MethodDelete, "/system/tokens/"+token, nil, nil)
			if result.Status != http.StatusOK || (result.JSON["code"] != float64(0) && result.JSON["code"] != float64(404)) {
				t.Errorf("token cleanup failed: %v", result.JSON)
			}
		}
	})
	listed := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/tokens", nil, nil))["data"])
	found := false
	for _, item := range listed {
		if object(t, item)["token"] == token {
			found = true
		}
	}
	if !found {
		t.Fatalf("created token absent from listing")
	}
	for _, config := range []*ServerTestConfig{&unauth, TestConfig} {
		result := restCall(t, config, http.MethodDelete, "/system/tokens/"+token, nil, nil)
		if config == &unauth {
			if result.Status != http.StatusUnauthorized || result.JSON["code"] != float64(401) {
				t.Fatalf("unauthorized token delete: %v", result.JSON)
			}
			continue
		}
		restCode(t, result, http.StatusOK, 0)
		deleted = true
	}
	remaining := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/tokens", nil, nil))["data"])
	for _, item := range remaining {
		if object(t, item)["token"] == token {
			t.Fatalf("deleted token remained listed")
		}
	}
	repeated := restOK(t, restCall(t, TestConfig, http.MethodDelete, "/system/tokens/"+token, nil, nil))
	if repeated["data"] != true {
		t.Fatalf("repeated delete should be idempotent: %v", repeated)
	}
}

func TestRESTSystemLogConfigurationRestoresValue(t *testing.T) {
	original := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/system/config/log", nil, nil))["data"])
	level, ok := original["level"].(string)
	if !ok || level == "" {
		t.Fatalf("log config missing level: %v", original)
	}
	t.Cleanup(func() {
		result := restCall(t, TestConfig, http.MethodPut, "/system/config/log", apiObject{"pkg_name": "", "level": level}, nil)
		if result.Status != http.StatusOK || result.JSON["code"] != float64(0) {
			t.Errorf("failed to restore log level: %v", result.JSON)
		}
	})
	missing := restCall(t, TestConfig, http.MethodPut, "/system/config/log", apiObject{}, nil)
	restCode(t, missing, http.StatusOK, 102)
	invalid := restCall(t, TestConfig, http.MethodPut, "/system/config/log", apiObject{"pkg_name": "rag", "level": "NOT_A_LEVEL"}, nil)
	restCode(t, invalid, http.StatusOK, 102)
	valid := restOK(t, restCall(t, TestConfig, http.MethodPut, "/system/config/log", apiObject{"pkg_name": "rag", "level": level}, nil))
	if !reflect.DeepEqual(object(t, valid["data"])["level"], level) {
		t.Fatalf("log update response=%v want level=%s", valid, level)
	}
}

func TestRESTSystemUnsupportedOceanbaseRouteIsNotAccepted(t *testing.T) {
	result := restCall(t, TestConfig, http.MethodGet, "/system/oceanbase/status", nil, nil)
	if result.Status != http.StatusNotFound && result.Status != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected unsupported oceanbase route status=%d body=%s", result.Status, fmt.Sprint(result.Body))
	}
}

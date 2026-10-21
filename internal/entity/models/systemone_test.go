package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newSystemOneForTest(baseURL string) *SystemOneModel {
	return NewSystemOneModel(
		map[string]string{"default": baseURL},
		URLSuffix{Decision: "systemone", Models: "models"},
	)
}

func TestSystemOneName(t *testing.T) {
	if got := newSystemOneForTest("http://unused").Name(); got != "SystemOne" {
		t.Errorf("Name()=%q", got)
	}
}

func TestSystemOneFactoryRegistration(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("systemone", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	model, ok := driver.(*SystemOneModel)
	if !ok {
		t.Fatalf("driver type=%T, want *SystemOneModel", driver)
	}
	if _, ok := model.NewInstance(map[string]string{"default": "http://other"}).(*SystemOneModel); !ok {
		t.Fatal("NewInstance did not return *SystemOneModel")
	}
}

// A System One endpoint has no chat shape; answering with an empty ChatResponse
// would look like a model that replied with nothing rather than a wrong choice.
func TestSystemOneRefusesChat(t *testing.T) {
	modelName := "decision-model-preview"
	if _, err := newSystemOneForTest("http://unused").ChatWithMessages(
		context.Background(), modelName, nil, nil, nil, nil,
	); err == nil {
		t.Fatal("ChatWithMessages did not refuse")
	}
}

// The model must not be offered before the base URL answers: the list callback
// only runs once an address is entered, and adding a model to a provider that
// cannot serve it fails much later, on the first decision call.
func TestSystemOneListModelsProbesBaseURL(t *testing.T) {
	withSSRFBypass(t)

	probed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probed = true
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   []map[string]any{{"id": "qwen-flash", "object": "model"}},
		})
	}))
	defer srv.Close()

	apiKey := "sk-test"
	models, err := newSystemOneForTest(srv.URL+"/v1").ListModels(
		context.Background(), &APIConfig{ApiKey: &apiKey},
	)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if !probed {
		t.Fatal("ListModels did not probe the base URL")
	}
	// Exactly one model, whatever the host's catalog advertises: the catalog is
	// not this protocol's model list, and everything in it would otherwise show
	// up as a decision model.
	if len(models) != 1 || models[0].Name != "decision-model-preview" {
		t.Fatalf("models=%v", models)
	}
	if len(models[0].ModelTypes) != 1 || models[0].ModelTypes[0] != "decision" {
		t.Errorf("types=%v, want [decision]", models[0].ModelTypes)
	}
}

// An unreachable base URL must fail the listing rather than fall back to the
// static catalog, or the model would appear even though it cannot be called.
func TestSystemOneListModelsFailsWhenProbeFails(t *testing.T) {
	withSSRFBypass(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	apiKey := "sk-test"
	models, err := newSystemOneForTest(srv.URL+"/v1").ListModels(
		context.Background(), &APIConfig{ApiKey: &apiKey},
	)
	if err == nil {
		t.Fatalf("ListModels returned %v, want an error", models)
	}
	if models != nil {
		t.Errorf("models=%v, want nil on failure", models)
	}
}

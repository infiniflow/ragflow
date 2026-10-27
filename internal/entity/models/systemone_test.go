package models

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// A decision model that cannot answer must not be addable: verification is the
// last check before the model is saved, and it is the only one that speaks the
// decision endpoint.
//
// The request is a well-formed one on purpose. A malformed request is rejected
// before the transport call, so a test that sent one would pass here without the
// server ever being asked — and would stay green if verification's status
// handling broke. The "asked" counter below makes that impossible.
func TestSystemOneDecideFailsWhenEndpointRejects(t *testing.T) {
	withSSRFBypass(t)

	for _, tc := range []struct {
		name   string
		status int
	}{
		{"rejected credential", http.StatusUnauthorized},
		{"no decision endpoint", http.StatusNotFound},
		{"server error", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				asked = true
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"no"}`))
			}))
			defer srv.Close()

			apiKey := "sk-test"
			modelName := "decision-model-preview"
			resp, err := newSystemOneForTest(srv.URL+"/v1").Decide(
				context.Background(), &modelName, &DecideRequest{
					State:     map[string]string{"passage": ""},
					Questions: map[string]Question{"probe": Noul{Instructions: "Is this relevant?"}},
				}, &APIConfig{ApiKey: &apiKey}, nil,
			)
			if err == nil {
				t.Fatalf("Decide accepted status %d", tc.status)
			}
			if resp != nil {
				t.Errorf("Decide returned a response alongside the error")
			}
			if !asked {
				t.Fatal("the endpoint was never asked, so this proves nothing about its answer")
			}
		})
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

func TestSystemOneDecide(t *testing.T) {
	for _, path := range []string{"/v1", "/v1/systemone/"} {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected decision endpoint or credentials")
				}
				var request struct {
					Model     string                                         `json:"model"`
					State     map[string]string                              `json:"state"`
					Questions map[string]struct{ Type, Instructions string } `json:"questions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Model != "decision-model-preview" || request.State["query"] != "hello" || request.Questions["greeting"].Type != "noul" || request.Questions["greeting"].Instructions != "Is this a greeting?" {
					t.Errorf("incorrect decision payload: %+v", request)
				}
				_, _ = w.Write([]byte(`{"answers":{"greeting":{"type":"noul","noul":0}},"usage":{"input_tokens":12}}`))
			}))
			defer srv.Close()
			key, name := "test-key", "decision-model-preview"
			result, err := newSystemOneForTest(srv.URL+path).Decide(t.Context(), &name, &DecideRequest{
				State: map[string]string{"query": "hello"}, Questions: map[string]Question{"greeting": Noul{Instructions: "Is this a greeting?"}},
			}, &APIConfig{ApiKey: &key}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.Answers["greeting"].Noul == nil || *result.Answers["greeting"].Noul != 0 || result.Usage.InputTokens != 12 {
				t.Fatalf("unexpected response: %+v", result)
			}
		})
	}
}

func TestSystemOneDecideValidation(t *testing.T) {
	model := newSystemOneForTest("http://unused")
	key, name := "test-key", "decision-model-preview"
	for _, count := range []int{0, MaxDecisionQuestions + 1} {
		questions := make(map[string]Question, count)
		for i := 0; i < count; i++ {
			questions[string(rune('a'+i))] = Noul{Instructions: "Question"}
		}
		if _, err := model.Decide(t.Context(), &name, &DecideRequest{Questions: questions}, &APIConfig{ApiKey: &key}, nil); err == nil {
			t.Fatalf("accepted %d questions", count)
		}
	}
}

func TestSystemOneDecideDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	key, name := "test-key", "decision-model-preview"
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := newSystemOneForTest(srv.URL).Decide(ctx, &name, &DecideRequest{Questions: map[string]Question{"a": Noul{Instructions: "Question"}}}, &APIConfig{ApiKey: &key}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Decide = %v; want deadline exceeded", err)
	}
}

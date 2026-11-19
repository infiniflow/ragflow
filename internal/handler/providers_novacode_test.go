package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity/models"
)

func useNovaCodeHandlerCatalog(t *testing.T) {
	t.Helper()
	provider := dao.GetModelProviderManager().FindProvider("NovaCode")
	if provider == nil {
		t.Fatal("NovaCode provider is not configured")
	}
	original := provider.Models
	provider.Models = []*models.Model{
		{Name: "nova-handler-visible", ModelTypes: []string{"chat", "vision"}},
		{Name: "nova-handler-hidden", ModelTypes: []string{"chat", "vision"}},
	}
	t.Cleanup(func() { provider.Models = original })
}

func TestFetchNovaCodeModelsScopesCatalogToAccessibleModels(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	useNovaCodeHandlerCatalog(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer sk-handler-test" {
				t.Error("model discovery did not send the configured key")
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"NOVA-HANDLER-VISIBLE"},{"id":"nova-handler-remote"}]}`))
		case "/api/v1/public/models":
			if r.Header.Get("Authorization") != "" {
				t.Error("public capability discovery must not send the API key")
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"groups":[{"models":[{"id":"nova-handler-visible","type":"text","metadata":{"mode":"chat","supports_vision":false}},{"id":"nova-handler-hidden","type":"text","metadata":{"mode":"chat","supports_vision":true}}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	list, err := FetchProviderModels(context.Background(), "NovaCode", "sk-handler-test", server.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	got := indexProviderModels(t, list)
	if len(got) != 2 || got["nova-handler-visible"] == nil || got["nova-handler-remote"] == nil {
		t.Fatalf("accessible model list = %v, want only the two live IDs", got)
	}
	if _, exists := got["nova-handler-hidden"]; exists {
		t.Fatal("static catalog exposed a model absent from the key's live list")
	}
	if types := providerModelMapTypes(got["nova-handler-visible"]); !reflect.DeepEqual(types, []string{"chat"}) {
		t.Fatalf("model types = %v, want authoritative remote chat without catalog vision", types)
	}
}

func TestFetchNovaCodeModelsRejectsEmptySupportedLiveList(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	useNovaCodeHandlerCatalog(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"groups":[]}}`))
	}))
	defer server.Close()

	list, err := FetchProviderModels(context.Background(), "NovaCode", "sk-handler-test", server.URL+"/v1", "")
	var coded *common.CodedError
	if !errors.As(err, &coded) || coded.Code != common.CodeDataError || !strings.Contains(coded.Message, "No supported NovaCode") {
		t.Fatalf("error = %v, want a clear unsupported model list error", err)
	}
	if len(list) != 0 {
		t.Fatalf("empty supported live list exposed static models: %v", list)
	}
}

func TestFetchNovaCodeModelsKeepsStaticFallback(t *testing.T) {
	useNovaCodeHandlerCatalog(t)
	list, err := FetchProviderModels(context.Background(), "NovaCode", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("model count = %d, want catalog fallback", len(list))
	}
}

func TestFetchNovaCodeModelsKeepsStaticFallbackOnDiscoveryFailure(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	useNovaCodeHandlerCatalog(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary upstream failure", http.StatusBadGateway)
	}))
	defer server.Close()

	list, err := FetchProviderModels(context.Background(), "NovaCode", "sk-handler-test", server.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("model count = %d, want catalog fallback", len(list))
	}
}

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestNovaCodeCapabilityPriority(t *testing.T) {
	previous := GetProviderManager()
	setProviderManager(&ProviderManager{Providers: []Provider{{Name: "NovaCode", Models: []*Model{
		{Name: "known-model", Alias: []string{"known-alias"}, ModelTypes: []string{"chat", "vision"}},
	}}}, AllModels: []Model{{Name: "other-provider-model", ModelTypes: []string{"chat", "vision"}}}, Alias2ModelIndex: map[string]int{"other-provider-model": 0}})
	t.Cleanup(func() { setProviderManager(previous) })
	for _, tc := range []struct {
		name, id string
		declared map[string][]string
		want     []string
		inferred bool
	}{
		{"explicit vision false wins", "known-model", map[string][]string{"known-model": {"chat"}}, []string{"chat"}, false},
		{"provider catalog", "known-model", nil, []string{"chat", "vision"}, false},
		{"confirmed alias", "known-alias", nil, []string{"chat", "vision"}, false},
		{"explicit new model", "new-model", map[string][]string{"new-model": {"chat", "vision"}}, []string{"chat", "vision"}, false},
		{"unrelated catalog is not evidence", "other-provider-model", nil, []string{"chat"}, true},
		{"unknown GPT is not assumed visual", "gpt-99-future", nil, []string{"chat"}, true},
		{"name hint fallback", "new-vl-model", nil, []string{"chat", "vision"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := parseNovaCodeModels(ModelList{Models: []ModelListItem{{ID: tc.id}}}, tc.declared)
			if len(models) != 1 || !reflect.DeepEqual(models[0].ModelTypes, tc.want) || models[0].ModelTypesInferred != tc.inferred {
				t.Fatalf("models=%+v want types=%v inferred=%v", models, tc.want, tc.inferred)
			}
		})
	}
}

func TestNovaCodeCapabilityDiscovery(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer test-secret" {
				t.Error("model listing requires the instance credential")
			}
			fmt.Fprint(w, `{"data":[{"id":"new-model"},{"id":"group-dependent"},{"id":"picture-generator"}]}`)
		case "/api/v1/public/models":
			if r.Header.Get("Authorization") != "" {
				t.Error("public directory must not receive the credential")
			}
			fmt.Fprint(w, `{"code":0,"data":{"groups":[{"models":[
				{"id":"new-model","type":"text","metadata":{"mode":"chat","supports_vision":true}},
				{"id":"not-in-key","type":"text","metadata":{"mode":"chat","supports_vision":true}},
				{"id":"group-dependent","type":"text","metadata":{"mode":"chat","supports_vision":true}},
				{"id":"picture-generator","type":"image"}
			]},{"models":[{"id":"group-dependent","type":"text","metadata":{"mode":"chat","supports_vision":false}}]}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
	models, err := driver.ListModels(t.Context(), novaCodeTestConfig(t, "responses", "medium"))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Name != "new-model" || !reflect.DeepEqual(models[0].ModelTypes, []string{"chat", "vision"}) || models[0].ModelTypesInferred {
		t.Fatalf("model access or vision metadata lost: %+v", models)
	}
	if models[1].Name != "group-dependent" || !reflect.DeepEqual(models[1].ModelTypes, []string{"chat"}) {
		t.Fatalf("conflicting groups must not advertise extra capabilities: %+v", models[1])
	}
}

func TestNovaCodeCapabilityDirectoryFallback(t *testing.T) {
	withSSRFBypass(t)
	previous := GetProviderManager()
	setProviderManager(&ProviderManager{Providers: []Provider{{Name: "NovaCode", Models: []*Model{{Name: "known-model", ModelTypes: []string{"chat", "vision"}}}}}})
	t.Cleanup(func() { setProviderManager(previous) })
	for _, response := range []string{`not JSON`, `{"code":1}`, `{"code":0,"data":{"groups":[]}}`, `{"code":0,"data":{"groups":[{"models":[{"id":"known-model","type":"text","metadata":{"mode":"chat","supports_vision":null}}]}]}}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					fmt.Fprint(w, `{"data":[{"id":"known-model"}]}`)
					return
				}
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
			models, err := driver.ListModels(t.Context(), novaCodeTestConfig(t, "responses", "medium"))
			if err != nil || len(models) != 1 || !reflect.DeepEqual(models[0].ModelTypes, []string{"chat", "vision"}) || models[0].ModelTypesInferred {
				t.Fatalf("catalog fallback failed: models=%+v error=%v", models, err)
			}
		})
	}
}

func TestNovaCodeCapabilityDirectoryCancellation(t *testing.T) {
	withSSRFBypass(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"known-model"}]}`)
			return
		}
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
	_, err := driver.ListModels(ctx, novaCodeTestConfig(t, "responses", "medium"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context canceled", err)
	}
}

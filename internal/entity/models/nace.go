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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// NaceModel drives Nace (https://console.nace.ai/v1), which serves the Drex
// decision models (drex-max, drex-v2.0) over the TypeSafe System One protocol.
// The embedded SystemOneModel already speaks the decide call; this driver adds
// the catalog, because GET /models on Nace answers {"models": [{"name": ...}]}
// rather than the OpenAI {"data": [{"id": ...}]} shape the generic lister
// expects, and the generic lister rejects that body as malformed.
type NaceModel struct {
	*SystemOneModel
}

// NewNaceModel creates a Nace driver.
func NewNaceModel(baseURL map[string]string, urlSuffix URLSuffix) *NaceModel {
	m := &NaceModel{SystemOneModel: NewSystemOneModel(baseURL, urlSuffix)}
	client := *m.baseModel.httpClient
	client.CheckRedirect = refuseCrossOriginRedirect
	m.baseModel.httpClient = &client
	return m
}

// SameOrigin reports whether two URLs share scheme, host and port (default
// ports are normalised). Unparseable URLs are never the same origin.
func SameOrigin(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return false
	}
	return sameURLOrigin(ua, ub)
}

func sameURLOrigin(a, b *url.URL) bool {
	if a.Hostname() == "" || b.Hostname() == "" {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		originPort(a) == originPort(b)
}

func originPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

// refuseCrossOriginRedirect stops a redirect from carrying the Authorization
// header to another scheme, host or port. Go itself keeps the header for the
// same hostname on a different port or scheme and for subdomains.
func refuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.Header.Get("Authorization") != "" && len(via) > 0 && !sameURLOrigin(req.URL, via[0].URL) {
		return fmt.Errorf("refusing redirect to %s://%s with credentials", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// Name returns the driver name.
func (m *NaceModel) Name() string {
	return "nace"
}

// NewInstance returns a driver bound to the given base URL.
func (m *NaceModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNaceModel(baseURL, m.baseModel.URLSuffix)
}

// naceModelList is the body of GET /models on Nace. alias_for is set on names
// that only point to another model (drex-latest, and retired versions served
// by a newer one).
type naceModelList struct {
	Models []struct {
		Name     string  `json:"name"`
		AliasFor *string `json:"alias_for"`
	} `json:"models"`
}

// ListModels asks which models the key can call and returns them as decision
// models; every model Nace serves is one.
//
// Aliases are skipped so a user adds the model that is actually billed and
// reported, not a pointer that moves to a newer version on an announced date.
// The catalog answers 401 without a valid key, which is why the web gates this
// listing on the key, as it does for SystemOne.
func (m *NaceModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doGetRequest(ctx, joinURL(baseURL, m.baseModel.URLSuffix.Models), apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, fmt.Errorf("nace: %w", err)
	}
	var list naceModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("nace: decode model list: %w", err)
	}
	if list.Models == nil {
		return nil, fmt.Errorf("nace: invalid model list format")
	}
	pm := GetProviderManager()
	var models []ListModelResponse
	for _, model := range list.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" || model.AliasFor != nil {
			continue
		}
		response := ListModelResponse{Name: name, ModelTypes: []string{"decision"}}
		if pm != nil {
			if entity, err := pm.GetModelByName("Nace", name); err == nil && entity != nil {
				response.ContextLength = entity.ContextLength
			}
		}
		models = append(models, response)
	}
	return models, nil
}

// CheckConnection verifies the key against the model catalog. The embedded
// driver's check goes through the generic OpenAI-shaped lister, which cannot
// read Nace's catalog, so it is routed through ListModels here.
func (m *NaceModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}

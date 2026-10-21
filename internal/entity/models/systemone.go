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
	"fmt"

	"ragflow/internal/common"
)

// systemOneDefaultSuffix is the decision endpoint this protocol uses, declared
// in the provider catalog as url_suffix.decision.
//
// It is declarative right now: nothing in this tree calls the decision endpoint,
// because the judge that speaks it lives in the agentic-RAG arm, which builds
// the URL from the instance's own base_url. The field exists so the catalog
// states where the endpoint is, and so a future Decide here has it.
const systemOneDefaultSuffix = "systemone"

// systemOneDefaultModelsSuffix lists the models the configured deployment
// serves; it is the standard OpenAI-shaped path, which this API reuses.
const systemOneDefaultModelsSuffix = "models"

// SystemOneModel implements ModelDriver for endpoints speaking TypeSafe System
// One: one request carrying several questions, answered in a single round trip.
//
// It embeds OpenAIAPICompatibleModel only to satisfy the rest of the (wide)
// ModelDriver interface — including Decide, which stays the "no such method"
// stub every other driver has. The judge that actually speaks this protocol
// lives in the agentic-RAG arm (decision-arm, internal/agentic_rag), which reads
// the configured model straight from the database; a second implementation here
// would be a second place for the request shape to drift.
//
// What this driver is for is REGISTRATION: making the provider appear in the
// picker, and making its model appear once a base URL has been shown to work.
type SystemOneModel struct {
	*OpenAIAPICompatibleModel
}

// NewSystemOneModel creates a System One provider instance.
//
// Missing URL suffixes are filled in here rather than at each call site: a
// catalog entry that forgets them would otherwise send the decide call to the
// bare host and the model list to the base URL, both of which answer 404 with no
// hint about which suffix was missing.
func NewSystemOneModel(baseURL map[string]string, urlSuffix URLSuffix) *SystemOneModel {
	if urlSuffix.Decision == "" {
		urlSuffix.Decision = systemOneDefaultSuffix
	}
	if urlSuffix.Models == "" {
		urlSuffix.Models = systemOneDefaultModelsSuffix
	}
	return &SystemOneModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the provider identifier.
func (m *SystemOneModel) Name() string {
	return "SystemOne"
}

// NewInstance creates an instance bound to another base URL.
func (m *SystemOneModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSystemOneModel(baseURL, m.baseModel.URLSuffix)
}

// systemOneModelName is the model this protocol serves.
//
// It is declared here rather than discovered because the deployment's /models
// does not list it: that endpoint returns the host's general model catalog (178
// entries for the reference deployment, none of them a decision model). Asking
// for the catalog would therefore either surface no decision model at all or
// offer the whole chat catalog as decision models — both wrong.
const systemOneModelName = "decision-model-preview"

// ListModels proves the configured base URL actually works, then returns the
// model this provider can drive.
//
// The probe is the point: the callback is only called once a base URL is
// entered, so the model appears — and can be added — only after the address has
// been shown to answer with the supplied credential. Returning the model
// without probing would let a user add a model to a provider that cannot serve
// it, and the failure would surface much later, on the first decision call.
func (m *SystemOneModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if _, err := m.VllmModel.ListModels(ctx, apiConfig); err != nil {
		return nil, err
	}
	return []ListModelResponse{{
		Name:       systemOneModelName,
		ModelTypes: []string{"decision"},
	}}, nil
}

// ChatWithMessages refuses rather than forwarding to /chat/completions.
//
// The endpoint has no chat shape. Forwarding would surface the server's 404 as
// an opaque transport error, and a caller that ignores the error would read the
// empty body as an empty answer. Failing here names the actual mistake: this
// model belongs in the decision slot, not the chat one.
func (m *SystemOneModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("systemone: %q is a decision model and cannot answer chat completions; select it as the decision model instead", modelName)
}

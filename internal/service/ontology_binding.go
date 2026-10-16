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

package service

import (
	"encoding/json"
	"strings"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/entity"
)

// ontologyBindingKey is the prompt_config / kwarg key that carries the
// knowledge-layer binding.
const ontologyBindingKey = "ontology"

// ontologyBindingPayload is the binding as a request, a template or an assistant
// writes it.
//
// Enabled exists so the precedence chain has a way to say NO. Without it a
// deployment default declared on a template could never be turned off for one
// assistant, and the only fix would be to edit the file every other assistant
// depends on.
type ontologyBindingPayload struct {
	ModelID             string   `json:"model_id"`
	ModelVersion        int      `json:"model_version"`
	KnowledgeDatasetIDs []string `json:"knowledge_dataset_ids"`
	Enabled             *bool    `json:"enabled"`
}

// OntologyBindingFor resolves this turn's knowledge-layer binding.
//
// Precedence is request → template → assistant, most specific first. A request
// that names a model is a caller overriding both defaults for one question; a
// template's declaration is the deployment's choice of what this agent should
// read; the assistant's setting is what the person using it chose.
//
// The resolved binding is complete or it is nothing. A model without a
// knowledge scope has nowhere to look and a scope without a model has no
// declarations to read, so a partial payload is reported (through the missing
// half) rather than silently completed from a lower layer — a half-configured
// override would otherwise mix one layer's model with another's scope.
func OntologyBindingFor(kwargs map[string]interface{}, promptConfig entity.JSONMap, templateID string) *agentic_rag.OntologyRunConfig {
	if payload, ok := ontologyBindingPayloadFrom(kwargs[ontologyBindingKey]); ok {
		return payload.toRunConfig()
	}
	if fromTemplate := agentic_rag.TemplateOntologyFor(templateID); fromTemplate != nil {
		return fromTemplate
	}
	if payload, ok := ontologyBindingPayloadFrom(promptConfig[ontologyBindingKey]); ok {
		return payload.toRunConfig()
	}
	return nil
}

// toRunConfig converts a payload, honouring an explicit disable and requiring
// both halves of the binding.
func (p ontologyBindingPayload) toRunConfig() *agentic_rag.OntologyRunConfig {
	if p.Enabled != nil && !*p.Enabled {
		return nil
	}
	cfg := &agentic_rag.OntologyRunConfig{
		ModelID:             strings.TrimSpace(p.ModelID),
		ModelVersion:        p.ModelVersion,
		KnowledgeDatasetIDs: dedupeStrings(p.KnowledgeDatasetIDs),
	}
	if !cfg.Enabled() {
		return nil
	}
	return cfg
}

// ontologyBindingPayloadFrom reads a binding from either a decoded object or the
// JSON text of one.
//
// Both shapes are accepted because the two callers arrive differently: an
// assistant's setting is stored as JSON in a text column and reaches here as a
// decoded map, while a request kwarg may be either depending on how the client
// serialised it. Accepting one and not the other would make the same
// configuration work in one entry point and vanish in the other.
func ontologyBindingPayloadFrom(raw interface{}) (ontologyBindingPayload, bool) {
	var payload ontologyBindingPayload
	switch v := raw.(type) {
	case nil:
		return payload, false
	case ontologyBindingPayload:
		return v, true
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return payload, false
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return payload, false
		}
		return payload, true
	case entity.JSONMap:
		data, err := json.Marshal(map[string]interface{}(v))
		if err != nil {
			return payload, false
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return payload, false
		}
		return payload, true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return payload, false
		}
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return payload, false
		}
		return payload, true
	default:
		return payload, false
	}
}

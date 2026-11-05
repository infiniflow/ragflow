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
	"testing"

	"ragflow/internal/entity"
)

// A decision model stored with model_type = 1<<7 must come back as the decision
// type. modelTypesFromBitmask is the "which types does this bitmask contain"
// helper, so an unlisted bit makesResolveModelType report that the model
// supports no type at all.
func TestModelTypesFromBitmaskIncludesDecision(t *testing.T) {
	got := modelTypesFromBitmask(entity.ModelTypeDecision)
	if len(got) != 1 || got[0] != entity.ModelTypeDecision {
		t.Fatalf("modelTypesFromBitmask(decision) = %v, want [decision]", got)
	}

	// A combined row keeps every type it declares, in declaration order.
	combined := entity.ModelTypeChat | entity.ModelTypeDecision
	got = modelTypesFromBitmask(combined)
	want := []entity.ModelType{entity.ModelTypeChat, entity.ModelTypeDecision}
	if len(got) != len(want) {
		t.Fatalf("modelTypesFromBitmask(chat|decision) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("modelTypesFromBitmask(chat|decision) = %v, want %v", got, want)
		}
	}
}

// The generic "default model of type X" path must resolve the tenant's decision
// model from the same two columns the model field map already uses.
func TestDefaultModelRefsIncludesDecision(t *testing.T) {
	name := "minimax-m3@cbn@OpenAI-API-Compatible"
	rowID := "d806c1155725406b840821e9e2897f01"
	membership := &entity.UserTenant{DecisionID: &name, TenantDecisionID: &rowID}

	gotName, gotID := defaultModelRefs(membership, entity.ModelTypeDecision)
	if gotName != name {
		t.Errorf("name = %q, want %q", gotName, name)
	}
	if gotID != rowID {
		t.Errorf("row id = %q, want %q", gotID, rowID)
	}

	if n, id := defaultModelRefs(nil, entity.ModelTypeDecision); n != "" || id != "" {
		t.Errorf("nil membership = (%q, %q), want empty", n, id)
	}
}

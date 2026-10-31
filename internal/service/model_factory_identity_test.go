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
	"errors"
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestModelFactoryIDAndNameShareValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		table   string
		updates map[string]interface{}
	}{
		{name: "active"},
		{name: "disabled model", table: "tenant_model", updates: map[string]interface{}{"status": "inactive"}},
		{name: "disabled instance", table: "tenant_model_instance", updates: map[string]interface{}{"status": "inactive"}},
		{name: "foreign tenant", table: "tenant_model_provider", updates: map[string]interface{}{"tenant_id": "foreign"}},
		{name: "foreign owner", table: "tenant_model_provider", updates: map[string]interface{}{"user_id": "foreign"}},
		{name: "wrong provider", table: "tenant_model_instance", updates: map[string]interface{}{"provider_id": "foreign"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelProviderServiceTestDB(t)
			useModelProviderServiceTestDB(t, db)
			seedModelProviderServiceScope(t, db)
			if test.table != "" {
				if err := db.Table(test.table).Where("1 = 1").Updates(test.updates).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := serviceTestContext(t, "user-1", "user-1")
			factory := NewModelFactory()
			access := ModelAccess{UserID: "user-1", TenantID: "user-1"}
			for _, ref := range []string{"model-1", "gpt-test@default@OpenAI"} {
				info, infoErr := factory.ResolveInfo(ctx, access, entity.ModelTypeChat, ref)
				types, typeErr := factory.ResolveModelType(ctx, access, ref)
				_, runErr := factory.resolveConfig(ctx, access, entity.ModelTypeChat, ref)
				for _, err := range []error{infoErr, typeErr, runErr} {
					if test.table != "" {
						if !errors.Is(err, errModelConfigUnavailable) {
							t.Fatalf("%s: error = %v, want unavailable", ref, err)
						}
					} else if err != nil {
						t.Fatalf("%s: %v", ref, err)
					}
				}
				if test.table == "" {
					if info.ID != "model-1" {
						t.Fatalf("%s: ID = %q", ref, info.ID)
					}
					if !reflect.DeepEqual(types, []entity.ModelType{entity.ModelTypeChat}) {
						t.Fatalf("%s: types = %v", ref, types)
					}
				}
			}
		})
	}
}

func TestModelFactoryMetadataDoesNotBuildExecutionConfig(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-1").Update("extra", "invalid JSON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.UserTenant{}).Where("id = ?", "user-tenant-1").Updates(map[string]interface{}{"llm_id": "gpt-test@default@OpenAI", "tenant_llm_id": "model-1"}).Error; err != nil {
		t.Fatal(err)
	}
	factory := NewModelFactory()
	access := ModelAccess{UserID: "user-1", TenantID: "user-1"}
	ctx := serviceTestContext(t, "user-1", "user-1")
	for _, ref := range []string{"model-1", "gpt-test@default@OpenAI"} {
		if _, err := factory.ResolveInfo(ctx, access, entity.ModelTypeChat, ref); err != nil {
			t.Fatalf("metadata %s: %v", ref, err)
		}
		if _, err := factory.ResolveModelType(ctx, access, ref); err != nil {
			t.Fatalf("types %s: %v", ref, err)
		}
		if _, err := factory.resolveConfig(ctx, access, entity.ModelTypeChat, ref); err == nil {
			t.Fatalf("execution %s accepted malformed configuration", ref)
		}
	}
	if _, err := factory.ResolveDefaultInfo(ctx, access, entity.ModelTypeChat); err != nil {
		t.Fatal(err)
	}
}

func TestModelFactoryCompositeSelectsRequestedType(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.Create(&entity.TenantModel{
		ID: "model-embedding", ProviderID: "provider-1", InstanceID: "instance-1", ModelName: "gpt-test", ModelType: int(entity.ModelTypeEmbedding), Status: "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	factory := NewModelFactory()
	access := ModelAccess{UserID: "user-1", TenantID: "user-1"}
	ctx := serviceTestContext(t, "user-1", "user-1")
	ref := "gpt-test@default@OpenAI"
	types, err := factory.ResolveModelType(ctx, access, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(types, []entity.ModelType{entity.ModelTypeChat, entity.ModelTypeEmbedding}) {
		t.Fatalf("types = %v", types)
	}
	info, err := factory.ResolveInfo(ctx, access, entity.ModelTypeEmbedding, ref)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "model-embedding" {
		t.Fatalf("selected model = %q", info.ID)
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-embedding").Update("status", "inactive").Error; err != nil {
		t.Fatal(err)
	}
	for _, modelRef := range []string{"model-embedding", ref} {
		if _, err := factory.ResolveInfo(ctx, access, entity.ModelTypeEmbedding, modelRef); !errors.Is(err, errModelConfigUnavailable) {
			t.Fatalf("%s: disabled model error = %v", modelRef, err)
		}
	}
}

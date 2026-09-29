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

package admin

import (
	"context"
	"path/filepath"
	"testing"

	"ragflow/internal/entity/models"
)

// TestListModelProvidersReturnsProviderCatalog guards the admin
// /providers endpoint behind the CLI's `LIST AVAILABLE PROVIDERS`
// command: it must return the provider catalog, not a stub error.
func TestListModelProvidersReturnsProviderCatalog(t *testing.T) {
	if err := models.InitProviderManager(filepath.Join("..", "..", "conf", "models")); err != nil {
		t.Fatalf("init provider manager: %v", err)
	}

	providers, err := (&Service{}).ListModelProviders(context.Background())
	if err != nil {
		t.Fatalf("ListModelProviders() error = %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("ListModelProviders() returned no providers")
	}

	for _, provider := range providers {
		if _, hasError := provider["error"]; hasError {
			t.Fatalf("ListModelProviders() returned an error entry: %v", provider)
		}
		name, ok := provider["name"].(string)
		if !ok || name == "" {
			t.Fatalf("provider entry without a name: %v", provider)
		}
		if _, ok := provider["model_types"]; !ok {
			t.Fatalf("provider %s entry without model_types: %v", name, provider)
		}
	}
}

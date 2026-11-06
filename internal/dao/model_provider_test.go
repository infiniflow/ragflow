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

package dao

import (
	"sync"
	"testing"

	"ragflow/internal/entity/models"
)

// TestGetModelProviderManagerServesPublishedCatalog pins that the getter reads
// the catalog the models package publishes and returns the same immutable
// snapshot to concurrent callers. The concurrent block is what exercises the
// publication path under `-race`.
func TestGetModelProviderManagerServesPublishedCatalog(t *testing.T) {
	first := GetModelProviderManager()
	if first == nil {
		t.Fatal("GetModelProviderManager returned nil")
	}
	if published := models.GetProviderManager(); published != first {
		t.Fatal("GetModelProviderManager returned a catalog the models package did not publish")
	}

	const callers = 8
	results := make([]*models.ProviderManager, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = GetModelProviderManager()
		}()
	}
	wg.Wait()

	for i, got := range results {
		if got != first {
			t.Fatalf("caller %d got a different catalog pointer", i)
		}
	}
}

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

package dataset

import (
	"ragflow/internal/common"
	"ragflow/internal/service"
	"strings"
	"testing"
)

func TestSDKDatasetUnicodeCharacterLimit(t *testing.T) {
	for _, n := range []int{128, 129} {
		t.Run(strings.Repeat("数", n), func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertCreateDatasetTenant(t, "tenant-1")
			name := strings.Repeat("数", n)
			_, code, err := testDatasetCreateService(t).CreateDataset(t.Context(), &service.CreateDatasetRequest{Name: name, ParseType: new(1), ParserID: new("general")}, testDatasetSubject("tenant-1"))
			if (err == nil) != (n == 128) {
				t.Fatalf("create characters=%d code=%d err=%v", n, code, err)
			}
		})
		t.Run("update", func(t *testing.T) {
			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
			ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
			_, code, err := testDatasetUpdateService(t).UpdateDataset(ctx, "kb-1", "tenant-1", service.UpdateDatasetRequest{Name: new(strings.Repeat("资", n))})
			if (err == nil) != (n == 128) {
				t.Fatalf("update characters=%d code=%d err=%v", n, code, err)
			}
		})
	}
}
func TestSDKDatasetNegativePagerankDiagnostic(t *testing.T) {
	db := setupDatasetUpdateTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
	_, code, err := testDatasetUpdateService(t).UpdateDataset(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), "kb-1", "tenant-1", service.UpdateDatasetRequest{Pagerank: new(int64(-1))})
	if code != common.CodeDataError || err == nil || !strings.Contains(err.Error(), "greater than or equal to 0") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestSDKDatasetUnicodeDeduplicationLimit(t *testing.T) {
	for _, n := range []int{125, 128} {
		name := strings.Repeat("数", n)
		got, err := common.UniqueNameCharacters(name, 128, func(candidate string) (bool, error) { return candidate == name, nil })
		if n == 125 {
			if err != nil || got != name+"(1)" {
				t.Fatalf("name=%q err=%v", got, err)
			}
		} else if err == nil {
			t.Fatal("deduplicated name exceeded character limit")
		}
	}
}

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
	"errors"
	"testing"

	"ragflow/internal/entity"
)

func TestAPITokenLimitSetting(t *testing.T) {
	db := setupAPITokenLimitDB(t)
	if err := db.Create(&entity.SystemSettings{Name: "api_key.max_count", Value: "2", DataType: "integer", Source: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"first", "second"} {
		if err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: token}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&entity.SystemSettings{}).Where("name = ?", "api_key.max_count").Update("value", "1").Error; err != nil {
		t.Fatal(err)
	}
	err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: "third"})
	if !errors.Is(err, ErrAPITokenLimitReached) {
		t.Fatalf("lowered limit did not reject new key: %v", err)
	}
	keys, err := NewAPITokenDAO().GetByTenantID(t.Context(), db, "tenant-1")
	if err != nil || len(keys) != 2 {
		t.Fatalf("lowering deleted existing keys: keys=%d error=%v", len(keys), err)
	}
	for _, value := range []string{"0", "-1", "1.5", "invalid", "", "999999999999999999999999999"} {
		if err := db.Model(&entity.SystemSettings{}).Where("name = ?", "api_key.max_count").Update("value", value).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := GetAPITokenLimit(t.Context(), db); err == nil {
			t.Fatalf("accepted invalid stored limit %q", value)
		}
	}
	if err := db.Migrator().DropTable(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAPITokenLimit(t.Context(), db); err == nil {
		t.Fatal("settings database failure silently used default")
	}
}

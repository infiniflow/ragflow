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
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestSetAPITokenLimitValidatesFirstSaveAndUpdate(t *testing.T) {
	for _, name := range []string{"api_key.max_count", "API_KEY.MAX_COUNT", "Api_Key.Max_Count"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", name, existing), func(t *testing.T) {
				h, db := setupSetVariableHandler(t)
				if existing {
					if err := db.Create(&entity.SystemSettings{Name: name, Value: "16", DataType: "string", Source: "admin"}).Error; err != nil {
						t.Fatal(err)
					}
				}
				for _, value := range []string{"0", "-1", "1.5", `"invalid"`, `""`, "999999999999999999999999999"} {
					response := putVariable(t, h, fmt.Sprintf(`{"var_name":%q,"var_value":%s}`, name, value))
					if response.Code == int(common.CodeSuccess) {
						t.Fatalf("accepted invalid limit %s", value)
					}
				}
				response := putVariable(t, h, fmt.Sprintf(`{"var_name":%q,"var_value":24}`, name))
				if response.Code != int(common.CodeSuccess) {
					t.Fatalf("save valid limit: %+v", response)
				}
				var setting entity.SystemSettings
				if err := db.Where("name = ?", name).Take(&setting).Error; err != nil {
					t.Fatal(err)
				}
				wantType := "integer"
				if existing {
					wantType = "string"
				}
				if setting.Value != "24" || setting.DataType != wantType {
					t.Fatalf("unexpected saved setting: %+v", setting)
				}
			})
		}
	}
}

func TestSetAPITokenLimitValidatesResolvedName(t *testing.T) {
	h, db := setupSetVariableHandler(t)
	// Match MySQL's trailing-space-insensitive name comparison.
	if err := db.Exec(`DROP TABLE system_settings`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE system_settings (
		name TEXT PRIMARY KEY COLLATE RTRIM, source TEXT, data_type TEXT, value TEXT,
		create_time INTEGER, create_date DATETIME, update_time INTEGER, update_date DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.SystemSettings{Name: "api_key.max_count", Value: "16", DataType: "integer", Source: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"0", "-1", "1.5", `"invalid"`} {
		response := putVariable(t, h, fmt.Sprintf(`{"var_name":"api_key.max_count ","var_value":%s}`, value))
		if response.Code == int(common.CodeSuccess) {
			t.Fatalf("accepted invalid limit %s through resolved name", value)
		}
	}
	var stored entity.SystemSettings
	if err := db.Where("name = ?", "api_key.max_count").Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Value != "16" {
		t.Fatalf("invalid update changed saved limit: %s", stored.Value)
	}
	response := putVariable(t, h, `{"var_name":"api_key.max_count ","var_value":24}`)
	if response.Code != int(common.CodeSuccess) {
		t.Fatalf("valid update failed: %+v", response)
	}
}

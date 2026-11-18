package admin

import (
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func TestSetAPITokenLimitValidatesFirstSaveAndUpdate(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			h, db := setupSetVariableHandler(t)
			if existing {
				if err := db.Create(&entity.SystemSettings{Name: "api_key.max_count", Value: "16", DataType: "string", Source: "admin"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, value := range []string{"0", "-1", "1.5", `"invalid"`, `""`, "999999999999999999999999999"} {
				response := putVariable(t, h, `{"var_name":"api_key.max_count","var_value":`+value+`}`)
				if response.Code == int(common.CodeSuccess) {
					t.Fatalf("accepted invalid limit %s", value)
				}
			}
			response := putVariable(t, h, `{"var_name":"api_key.max_count","var_value":24}`)
			if response.Code != int(common.CodeSuccess) {
				t.Fatalf("save valid limit: %+v", response)
			}
			var setting entity.SystemSettings
			if err := db.Where("name = ?", "api_key.max_count").Take(&setting).Error; err != nil {
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

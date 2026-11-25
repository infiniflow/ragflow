package entity

import "testing"

func TestValidateSystemSettingValueChecksTypeOnly(t *testing.T) {
	for _, name := range []string{"api_key.max_count", "API_KEY.MAX_COUNT", "other.setting"} {
		if err := ValidateSystemSettingValue(SystemSettings{Name: name, DataType: "integer"}, "0"); err != nil {
			t.Fatalf("generic integer validation for %s rejected zero: %v", name, err)
		}
	}
}

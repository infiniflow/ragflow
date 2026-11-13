package handler

import "testing"

// RED: dataset create must accept pipeline_version_id so a specific (non-latest)
// canvas version can be pinned at bind time (design section 5 C2).
func TestDatasetCreateAllowsPipelineVersionID(t *testing.T) {
	if !createDatasetAllowedFields["pipeline_version_id"] {
		t.Fatalf("createDatasetAllowedFields must include pipeline_version_id; otherwise binding a pinned version is rejected as 'extra input'")
	}
}

// RED: dataset update must accept pipeline_version_id so an existing dataset can
// be re-bound to a different version (design section 5 C3 / sticky-snapshot switch).
func TestDatasetUpdateAllowsPipelineVersionID(t *testing.T) {
	if !updateDatasetAllowedFields["pipeline_version_id"] {
		t.Fatalf("updateDatasetAllowedFields must include pipeline_version_id; otherwise version re-bind is rejected as 'extra input'")
	}
}

// Adversarial: the whitelist must still reject genuinely unknown fields, proving
// the fix is a targeted allow-list addition and not a blanket "allow everything".
func TestDatasetAllowedFieldsRejectUnknownField(t *testing.T) {
	if createDatasetAllowedFields["__bogus_field__"] {
		t.Fatalf("createDatasetAllowedFields must not accept unknown fields")
	}
	if updateDatasetAllowedFields["__bogus_field__"] {
		t.Fatalf("updateDatasetAllowedFields must not accept unknown fields")
	}
}

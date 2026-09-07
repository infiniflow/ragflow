package service

import "testing"

func TestSystemServiceHealthzReportsUnhealthyDependencies(t *testing.T) {
	result, allOK := NewSystemService().Healthz(t.Context())
	if allOK {
		t.Fatal("allOK=true, want false without initialized dependencies")
	}
	if result.Status != "nok" {
		t.Fatalf("status=%q, want nok", result.Status)
	}
	if result.DB != "nok" || result.Redis != "nok" || result.DocEngine != "nok" || result.Storage != "nok" {
		t.Fatalf("unexpected health result: %+v", result)
	}
	if len(result.Meta) == 0 {
		t.Fatal("expected failure metadata")
	}
}

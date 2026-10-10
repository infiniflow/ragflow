//go:build cgo

package component

import "testing"

func TestIsAutoLayoutRecognize(t *testing.T) {
	if !isAutoLayoutRecognize("Auto") || !isAutoLayoutRecognize(" auto ") {
		t.Fatal("expected Auto to match")
	}
	if isAutoLayoutRecognize("DeepDOC") {
		t.Fatal("DeepDOC must not match Auto")
	}
}

func TestParsePositiveInt(t *testing.T) {
	n, err := parsePositiveInt("100")
	if err != nil || n != 100 {
		t.Fatalf("parsePositiveInt(100) = %d, %v", n, err)
	}
	if _, err := parsePositiveInt("0"); err == nil {
		t.Fatal("expected error for zero")
	}
}

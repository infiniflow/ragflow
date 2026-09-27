package models

import (
	"strings"
	"testing"
)

func TestReadModelResponseBodyLimited(t *testing.T) {
	body, err := readModelResponseBodyLimited(strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatalf("readModelResponseBodyLimited: %v", err)
	}
	if string(body) != "abcd" {
		t.Fatalf("body=%q, want abcd", string(body))
	}
}

func TestReadModelResponseBodyLimitedRejectsOversizedBody(t *testing.T) {
	_, err := readModelResponseBodyLimited(strings.NewReader("abcde"), 4)
	if err == nil || !strings.Contains(err.Error(), "response body exceeds 4 bytes") {
		t.Fatalf("err=%v, want response body exceeds 4 bytes", err)
	}
}

func TestReadModelResponseBodyLimitedRejectsInvalidLimit(t *testing.T) {
	_, err := readModelResponseBodyLimited(strings.NewReader("abcd"), 0)
	if err == nil || !strings.Contains(err.Error(), "response body limit must be positive") {
		t.Fatalf("err=%v, want positive limit error", err)
	}
}

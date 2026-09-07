package models

import (
	"strings"
	"testing"
)

func requireNoSuchMethod(t *testing.T, name string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected no such method error, got nil", name)
	}
	if !strings.Contains(err.Error(), "no such method") {
		t.Fatalf("%s: expected no such method error, got %v", name, err)
	}
}

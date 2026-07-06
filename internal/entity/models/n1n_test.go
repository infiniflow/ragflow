package models

import (
	"net/http"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newN1NForTest(baseURL string) *N1NModel {
	return NewN1NModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestN1NName(t *testing.T) {
	if got := newN1NForTest("http://unused").Name(); got != "n1n" {
		t.Errorf("Name()=%q, want %q", got, "n1n")
	}
}

func TestN1NFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("n1n", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*N1NModel); !ok {
		t.Fatalf("driver type=%T, want *N1NModel", driver)
	}
}

func TestN1NNewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewN1NModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewN1NModel returned nil")
	}
}

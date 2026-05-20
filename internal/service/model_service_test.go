package service

import (
	"strings"
	"testing"

	modelModule "ragflow/internal/entity/models"
)

type stubModelDriver struct {
	modelModule.ModelDriver
	newInstance func(map[string]string) modelModule.ModelDriver
}

var _ modelModule.ModelDriver = (*stubModelDriver)(nil)

func (s *stubModelDriver) NewInstance(baseURL map[string]string) modelModule.ModelDriver {
	if s.newInstance != nil {
		return s.newInstance(baseURL)
	}
	return s
}

func (s *stubModelDriver) Name() string {
	return "stub"
}

func TestNewModelDriverForBaseURLPreservesEmptyRegion(t *testing.T) {
	expected := &stubModelDriver{}
	var gotBaseURL map[string]string
	driver := &stubModelDriver{
		newInstance: func(baseURL map[string]string) modelModule.ModelDriver {
			gotBaseURL = baseURL
			return expected
		},
	}

	got, err := newModelDriverForBaseURL(driver, "stub", "", "http://localhost:1234")
	if err != nil {
		t.Fatalf("newModelDriverForBaseURL returned error: %v", err)
	}
	if got != expected {
		t.Fatalf("expected returned driver %p, got %p", expected, got)
	}
	if gotBaseURL[""] != "http://localhost:1234" {
		t.Fatalf("expected empty-region base URL, got %#v", gotBaseURL)
	}
	if _, ok := gotBaseURL["default"]; ok {
		t.Fatalf("unexpected default region key in base URL map: %#v", gotBaseURL)
	}
}

func TestNewModelDriverForBaseURLUsesProvidedRegion(t *testing.T) {
	var gotBaseURL map[string]string
	driver := &stubModelDriver{
		newInstance: func(baseURL map[string]string) modelModule.ModelDriver {
			gotBaseURL = baseURL
			return &stubModelDriver{}
		},
	}

	_, err := newModelDriverForBaseURL(driver, "stub", "cn-hangzhou", "http://localhost:5678")
	if err != nil {
		t.Fatalf("newModelDriverForBaseURL returned error: %v", err)
	}
	if gotBaseURL["cn-hangzhou"] != "http://localhost:5678" {
		t.Fatalf("expected regional base URL, got %#v", gotBaseURL)
	}
	if _, ok := gotBaseURL["default"]; ok {
		t.Fatalf("unexpected default region key in base URL map: %#v", gotBaseURL)
	}
}

func TestNewModelDriverForBaseURLSkipsEmptyBaseURL(t *testing.T) {
	for _, baseURL := range []string{"", "   "} {
		t.Run(baseURL, func(t *testing.T) {
			called := false
			driver := &stubModelDriver{
				newInstance: func(map[string]string) modelModule.ModelDriver {
					called = true
					return nil
				},
			}

			got, err := newModelDriverForBaseURL(driver, "deepseek", "default", baseURL)
			if err != nil {
				t.Fatalf("newModelDriverForBaseURL returned error: %v", err)
			}
			if got != driver {
				t.Fatalf("expected original driver %p, got %p", driver, got)
			}
			if called {
				t.Fatal("expected empty base URL to skip NewInstance")
			}
		})
	}
}

func TestNewModelDriverForBaseURLRejectsNilInstance(t *testing.T) {
	driver := &stubModelDriver{
		newInstance: func(map[string]string) modelModule.ModelDriver {
			return nil
		},
	}

	got, err := newModelDriverForBaseURL(driver, "deepseek", "default", "http://localhost:1234")
	if err == nil {
		t.Fatal("expected nil NewInstance result to return an error")
	}
	if got != nil {
		t.Fatalf("expected nil driver on error, got %T", got)
	}
	if !strings.Contains(err.Error(), "deepseek") || !strings.Contains(err.Error(), "custom base_url") {
		t.Fatalf("expected provider-specific custom base_url error, got %v", err)
	}
}

func TestNewModelDriverForBaseURLRejectsNilDriver(t *testing.T) {
	got, err := newModelDriverForBaseURL(nil, "deepseek", "default", "http://localhost:1234")
	if err == nil {
		t.Fatal("expected nil driver to return an error")
	}
	if got != nil {
		t.Fatalf("expected nil driver on error, got %T", got)
	}
	if !strings.Contains(err.Error(), "driver not found") {
		t.Fatalf("expected driver not found error, got %v", err)
	}
}

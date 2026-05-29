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

func TestUnsupportedModelDriverReturnsNoSuchMethod(t *testing.T) {
	driver := UnsupportedModelDriver{ProviderName: "test-provider"}
	modelName := "model"
	text := "hello"

	checks := []struct {
		name string
		call func() error
	}{
		{"Embed", func() error {
			_, err := driver.Embed(&modelName, []string{text}, nil, nil)
			return err
		}},
		{"Rerank", func() error {
			_, err := driver.Rerank(&modelName, text, []string{text}, nil, nil)
			return err
		}},
		{"TranscribeAudio", func() error {
			_, err := driver.TranscribeAudio(&modelName, &text, nil, nil)
			return err
		}},
		{"TranscribeAudioWithSender", func() error {
			return driver.TranscribeAudioWithSender(&modelName, &text, nil, nil, nil)
		}},
		{"AudioSpeech", func() error {
			_, err := driver.AudioSpeech(&modelName, &text, nil, nil)
			return err
		}},
		{"AudioSpeechWithSender", func() error {
			return driver.AudioSpeechWithSender(&modelName, &text, nil, nil, nil)
		}},
		{"OCRFile", func() error {
			_, err := driver.OCRFile(&modelName, nil, &text, nil, nil)
			return err
		}},
		{"ParseFile", func() error {
			_, err := driver.ParseFile(&modelName, nil, &text, nil, nil)
			return err
		}},
		{"Balance", func() error {
			_, err := driver.Balance(nil)
			return err
		}},
		{"CheckConnection", func() error {
			return driver.CheckConnection(nil)
		}},
		{"ListTasks", func() error {
			_, err := driver.ListTasks(nil)
			return err
		}},
		{"ShowTask", func() error {
			_, err := driver.ShowTask("task-id", nil)
			return err
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			requireNoSuchMethod(t, check.name, err)
			if !strings.Contains(err.Error(), "test-provider") {
				t.Fatalf("%s: expected provider name in error, got %v", check.name, err)
			}
		})
	}
}

func TestUnsupportedModelDriverNoSuchMethodErrorFormats(t *testing.T) {
	_, err := UnsupportedModelDriver{ProviderName: "test-provider"}.Balance(nil)
	if err == nil || err.Error() != "test-provider, no such method" {
		t.Fatalf("expected provider-prefixed error, got %v", err)
	}

	_, err = UnsupportedModelDriver{}.Balance(nil)
	if err == nil || err.Error() != "no such method" {
		t.Fatalf("expected bare no such method error, got %v", err)
	}
}

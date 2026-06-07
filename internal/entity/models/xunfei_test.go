package models

import "testing"

func TestXunFeiUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	driver := NewXunFeiModel(map[string]string{"default": "http://unused"}, URLSuffix{}).
		NewInstance(map[string]string{"default": "http://unused"})
	modelName := "spark"
	text := "hello"

	checks := []struct {
		name string
		call func() error
	}{
		{"Embed", func() error {
			_, err := driver.Embed(&modelName, []string{text}, &APIConfig{}, nil)
			return err
		}},
		{"Rerank", func() error {
			_, err := driver.Rerank(&modelName, text, []string{text}, &APIConfig{}, nil)
			return err
		}},
		{"TranscribeAudio", func() error {
			_, err := driver.TranscribeAudio(&modelName, &text, &APIConfig{}, nil)
			return err
		}},
		{"TranscribeAudioWithSender", func() error {
			return driver.TranscribeAudioWithSender(&modelName, &text, &APIConfig{}, nil, nil)
		}},
		{"AudioSpeech", func() error {
			_, err := driver.AudioSpeech(&modelName, &text, &APIConfig{}, nil)
			return err
		}},
		{"AudioSpeechWithSender", func() error {
			return driver.AudioSpeechWithSender(&modelName, &text, &APIConfig{}, nil, nil)
		}},
		{"OCRFile", func() error {
			_, err := driver.OCRFile(&modelName, nil, &text, &APIConfig{}, nil)
			return err
		}},
		{"ParseFile", func() error {
			_, err := driver.ParseFile(&modelName, nil, &text, &APIConfig{}, nil)
			return err
		}},
		{"Balance", func() error {
			_, err := driver.Balance(&APIConfig{})
			return err
		}},
		{"CheckConnection", func() error {
			return driver.CheckConnection(&APIConfig{})
		}},
		{"ListTasks", func() error {
			_, err := driver.ListTasks(&APIConfig{})
			return err
		}},
		{"ShowTask", func() error {
			_, err := driver.ShowTask("task-id", &APIConfig{})
			return err
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			requireNoSuchMethod(t, check.name, check.call())
		})
	}
}

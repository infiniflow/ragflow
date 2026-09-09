package service

import "testing"

func TestRelatedQuestionsConfigSkipsZeroTopP(t *testing.T) {
	cfg := relatedQuestionsConfig(map[string]interface{}{
		"llm_setting": map[string]interface{}{
			"temperature": float64(0.2),
			"top_p":       float64(0),
			"parameter":   map[string]interface{}{"unused": true},
		},
	})

	if cfg == nil || cfg.Temperature == nil || *cfg.Temperature != 0.2 {
		t.Fatalf("expected temperature 0.2, got %+v", cfg)
	}
	if cfg.TopP != nil {
		t.Fatalf("expected zero top_p to be omitted, got %v", *cfg.TopP)
	}
}

func TestParseRelatedQuestionsStandard(t *testing.T) {
	input := `1. How do electric vehicles impact the environment?
2. What are the advantages of owning an electric car?
3. What is the cost-effectiveness?`

	got := parseRelatedQuestions(input)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "How do electric vehicles impact the environment?" {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "What are the advantages of owning an electric car?" {
		t.Errorf("unexpected [1]: %q", got[1])
	}
	if got[2] != "What is the cost-effectiveness?" {
		t.Errorf("unexpected [2]: %q", got[2])
	}
}

func TestParseRelatedQuestionsEmpty(t *testing.T) {
	got := parseRelatedQuestions("")
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestParseRelatedQuestionsNoNumberedLines(t *testing.T) {
	input := `Here are some related questions:
- First question
- Second question`

	got := parseRelatedQuestions(input)
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestParseRelatedQuestionsMixedContent(t *testing.T) {
	input := `Here are some related questions:
1. First related question.
Some explanation text.
2. Second related question.
More text.
3. Third related question.`

	got := parseRelatedQuestions(input)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "First related question." {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "Second related question." {
		t.Errorf("unexpected [1]: %q", got[1])
	}
	if got[2] != "Third related question." {
		t.Errorf("unexpected [2]: %q", got[2])
	}
}

func TestParseRelatedQuestionsMultiDigit(t *testing.T) {
	input := `10. Tenth question.
11. Eleventh question.`

	got := parseRelatedQuestions(input)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0] != "Tenth question." {
		t.Errorf("unexpected [0]: %q", got[0])
	}
	if got[1] != "Eleventh question." {
		t.Errorf("unexpected [1]: %q", got[1])
	}
}

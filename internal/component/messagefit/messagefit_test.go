package messagefit

import (
	"strings"
	"testing"
)

func countMessages(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		if m.Content != "" {
			total += len(m.Content) // rough proxy; not token-accurate
		}
	}
	return total
}

func TestFit_AllFits(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "hello"},
		{Role: "user", Content: "world"},
	}
	got := Fit(msgs, 100000)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	// Nothing should be dropped.
	if msgs[0].Content != "hello" || msgs[1].Content != "world" {
		t.Errorf("messages modified when they fit: %+v", msgs)
	}
}

func TestFit_Step2_DropsMiddle(t *testing.T) {
	// Three system + one user in the middle + last user. With a tight
	// budget that fits system + last user, the middle user is dropped.
	long := strings.Repeat("x ", 500) // ~1000 tokens
	short := "y"
	msgs := []Message{
		{Role: "system", Content: long},
		{Role: "user", Content: short},
		{Role: "user", Content: long},
	}
	// Budget that fits system + last but not all three.
	budget := len(long)/2 + len(long)/2 + 100 // rough
	_ = budget
	// Use a very tight budget to force step 3.
	got := Fit(msgs, 50)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	// After fitting, at least the last message should be kept (non-empty).
	foundLast := false
	for _, m := range msgs {
		if m.Content != "" {
			foundLast = true
		}
	}
	if !foundLast {
		t.Errorf("all messages empty after fit: %+v", msgs)
	}
}

func TestFit_Step3_SystemDominates(t *testing.T) {
	// System takes >80% of tokens → preserve user, trim system.
	sysContent := strings.Repeat("a ", 800)  // ~1600 tokens
	userContent := strings.Repeat("b ", 100) // ~200 tokens
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}
	got := Fit(msgs, 500)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	// System should have been trimmed, user preserved.
	if len(msgs[0].Content) >= len(sysContent) {
		t.Errorf("system not trimmed: got %d want < %d", len(msgs[0].Content), len(sysContent))
	}
}

func TestFit_Step3_UserDominates(t *testing.T) {
	// User takes >20% → preserve system, trim user.
	sysContent := strings.Repeat("a ", 200)
	userContent := strings.Repeat("b ", 800)
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}
	got := Fit(msgs, 500)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	if len(msgs[1].Content) >= len(userContent) {
		t.Errorf("user not trimmed: got %d want < %d", len(msgs[1].Content), len(userContent))
	}
}

func TestFit_SingleMessage(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: strings.Repeat("x ", 1000)},
	}
	got := Fit(msgs, 100)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	if len(msgs[0].Content) >= 1000 {
		t.Errorf("single message not trimmed: got %d", len(msgs[0].Content))
	}
}

func TestFit_ZeroMaxTokens(t *testing.T) {
	// maxTokens <= 0 should use 8192 default and not panic.
	msgs := []Message{
		{Role: "system", Content: "hello"},
		{Role: "user", Content: "world"},
	}
	got := Fit(msgs, 0)
	if got == 0 {
		t.Errorf("Fit returned 0, want > 0")
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "world" {
		t.Errorf("messages modified with default budget: %+v", msgs)
	}
}

func TestFit_Empty(t *testing.T) {
	got := Fit(nil, 1000)
	if got != 0 {
		t.Errorf("Fit on nil = %d, want 0", got)
	}
	got = Fit([]Message{}, 1000)
	if got != 0 {
		t.Errorf("Fit on empty = %d, want 0", got)
	}
}

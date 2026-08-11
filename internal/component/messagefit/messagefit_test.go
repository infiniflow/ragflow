package messagefit

import (
	"strings"
	"testing"

	"ragflow/internal/tokenizer"
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
	// system + last user fit within the budget, but all three together do
	// not, so Step 2 drops the middle user and keeps system + last intact.
	sysContent := strings.Repeat("x ", 200)
	middle := "middle"
	last := "last"
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: middle},
		{Role: "user", Content: last},
	}
	budget := tokenizer.NumTokensFromString(sysContent) + tokenizer.NumTokensFromString(last)

	if got := Fit(msgs, budget); got == 0 {
		t.Fatalf("Fit returned 0, want > 0")
	}
	if msgs[1].Content != "" {
		t.Errorf("middle message not cleared: %q", msgs[1].Content)
	}
	if msgs[0].Content != sysContent || msgs[2].Content != last {
		t.Errorf("retained messages modified: %+v", msgs)
	}
}

// TestFit_ExactBudget verifies that a total exactly equal to the budget
// counts as fitting: nothing is dropped or trimmed.
func TestFit_ExactBudget(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "abc"},
		{Role: "user", Content: "def"},
	}
	budget := tokenizer.NumTokensFromString("abc") + tokenizer.NumTokensFromString("def")

	if got := Fit(msgs, budget); got == 0 {
		t.Fatalf("Fit returned 0, want > 0")
	}
	if msgs[0].Content != "abc" || msgs[1].Content != "def" {
		t.Errorf("messages modified at exact budget: %+v", msgs)
	}
}

// TestFit_SystemOnlyMessages verifies that when only system messages are
// retained (no non-system message), the whole budget is spread across every
// retained system message and the total never exceeds it.
func TestFit_SystemOnlyMessages(t *testing.T) {
	sys1 := strings.Repeat("s ", 800)
	sys2 := strings.Repeat("t ", 200)
	msgs := []Message{
		{Role: "system", Content: sys1},
		{Role: "system", Content: sys2},
	}
	const budget = 500

	if got := Fit(msgs, budget); got == 0 {
		t.Fatalf("Fit returned 0, want > 0")
	}
	total := tokenizer.NumTokensFromString(msgs[0].Content) + tokenizer.NumTokensFromString(msgs[1].Content)
	if total > budget {
		t.Errorf("fitted total %d exceeds budget %d", total, budget)
	}
	if len(msgs[0].Content) >= len(sys1) || len(msgs[1].Content) >= len(sys2) {
		t.Errorf("system messages not trimmed: %+v", msgs)
	}
}

// TestFit_TrimsAllSystemMessages verifies that the system budget is spread
// across every retained system message, not just the first one, so the final
// total never exceeds the budget.
func TestFit_TrimsAllSystemMessages(t *testing.T) {
	sys1 := strings.Repeat("s ", 800) // dominates the token count
	sys2 := strings.Repeat("t ", 200)
	last := "last"
	msgs := []Message{
		{Role: "system", Content: sys1},
		{Role: "system", Content: sys2},
		{Role: "user", Content: last},
	}
	const budget = 500

	if got := Fit(msgs, budget); got == 0 {
		t.Fatalf("Fit returned 0, want > 0")
	}
	if msgs[2].Content != last {
		t.Errorf("last user message not preserved: %q", msgs[2].Content)
	}
	total := tokenizer.NumTokensFromString(msgs[0].Content) +
		tokenizer.NumTokensFromString(msgs[1].Content) +
		tokenizer.NumTokensFromString(msgs[2].Content)
	if total > budget {
		t.Errorf("fitted total %d exceeds budget %d", total, budget)
	}
	if len(msgs[0].Content) >= len(sys1) || len(msgs[1].Content) >= len(sys2) {
		t.Errorf("system messages not trimmed: %+v", msgs)
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

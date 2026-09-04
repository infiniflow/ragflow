package tokenizer

import (
	"slices"
	"strings"
	"testing"
)

func TestFit_AllFits(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "hello"},
		{Role: "user", Content: "world"},
	}
	kept, keptIdx, count := Fit(msgs, 100000)
	if count == 0 {
		t.Errorf("Fit returned count 0, want > 0")
	}
	if len(kept) != 2 || !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("got kept=%+v keptIdx=%v, want both messages", kept, keptIdx)
	}
	if kept[0].Content != "hello" || kept[1].Content != "world" {
		t.Errorf("messages modified when they fit: %+v", kept)
	}
}

// TestFit_DoesNotMutateInput verifies the non-mutating contract: Fit never
// rewrites msgs, so a caller cannot accidentally send emptied entries.
func TestFit_DoesNotMutateInput(t *testing.T) {
	orig := []Message{
		{Role: "system", Content: strings.Repeat("s ", 500)},
		{Role: "user", Content: strings.Repeat("u ", 500)},
		{Role: "user", Content: "last"},
	}
	msgs := slices.Clone(orig)
	Fit(msgs, 100)
	if !slices.Equal(msgs, orig) {
		t.Fatalf("Fit mutated its input:\n got %+v\nwant %+v", msgs, orig)
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
	budget := NumTokensFromString(sysContent) + NumTokensFromString(last)

	kept, keptIdx, count := Fit(msgs, budget)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if !slices.Equal(keptIdx, []int{0, 2}) {
		t.Fatalf("keptIdx = %v, want [0 2] (middle dropped)", keptIdx)
	}
	if kept[0].Content != sysContent || kept[1].Content != last {
		t.Errorf("retained messages modified: %+v", kept)
	}
}

func TestFit_ExactBudget(t *testing.T) {
	// A total exactly equal to the budget counts as fitting.
	msgs := []Message{
		{Role: "system", Content: "abc"},
		{Role: "user", Content: "def"},
	}
	budget := NumTokensFromString("abc") + NumTokensFromString("def")

	kept, keptIdx, _ := Fit(msgs, budget)
	if len(kept) != 2 || !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("got kept=%+v keptIdx=%v, want both messages kept", kept, keptIdx)
	}
	if kept[0].Content != "abc" || kept[1].Content != "def" {
		t.Errorf("messages modified at exact budget: %+v", kept)
	}
}

// TestFit_NoSystem_KeepsOnlyLast locks the Python-parity behavior: without a
// system message, Step 2 keeps only the last non-system message.
func TestFit_NoSystem_KeepsOnlyLast(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: strings.Repeat("a ", 300)},
		{Role: "assistant", Content: strings.Repeat("b ", 300)},
		{Role: "user", Content: "last"},
	}
	kept, keptIdx, _ := Fit(msgs, 100)
	if !slices.Equal(keptIdx, []int{2}) {
		t.Fatalf("keptIdx = %v, want [2] (only the last user kept)", keptIdx)
	}
	if len(kept) != 1 || kept[0].Content != "last" {
		t.Fatalf("kept = %+v, want the last user message", kept)
	}
}

func TestFit_SystemOnlyMessages(t *testing.T) {
	sys1 := strings.Repeat("s ", 800)
	sys2 := strings.Repeat("t ", 200)
	msgs := []Message{
		{Role: "system", Content: sys1},
		{Role: "system", Content: sys2},
	}
	const budget = 500

	kept, keptIdx, count := Fit(msgs, budget)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if len(kept) != 2 || !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("got kept=%+v keptIdx=%v, want both systems", kept, keptIdx)
	}
	total := NumTokensFromString(kept[0].Content) + NumTokensFromString(kept[1].Content)
	if total > budget {
		t.Errorf("fitted total %d exceeds budget %d", total, budget)
	}
	fitted0 := NumTokensFromString(kept[0].Content)
	fitted1 := NumTokensFromString(kept[1].Content)
	if fitted0 == 0 || fitted1 == 0 {
		t.Errorf("a system message was emptied by fitting: %+v", kept)
	}
	if fitted0 >= NumTokensFromString(sys1) || fitted1 >= NumTokensFromString(sys2) {
		t.Errorf("system messages not trimmed: %+v", kept)
	}
}

func TestFit_TrimsAllSystemMessages(t *testing.T) {
	sys1 := strings.Repeat("s ", 800)
	sys2 := strings.Repeat("t ", 200)
	last := "last"
	msgs := []Message{
		{Role: "system", Content: sys1},
		{Role: "system", Content: sys2},
		{Role: "user", Content: last},
	}
	const budget = 500

	kept, keptIdx, count := Fit(msgs, budget)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if !slices.Equal(keptIdx, []int{0, 1, 2}) {
		t.Fatalf("keptIdx = %v, want [0 1 2]", keptIdx)
	}
	if kept[2].Content != last {
		t.Errorf("last user message not preserved: %q", kept[2].Content)
	}
	total := NumTokensFromString(kept[0].Content) +
		NumTokensFromString(kept[1].Content) +
		NumTokensFromString(kept[2].Content)
	if total > budget {
		t.Errorf("fitted total %d exceeds budget %d", total, budget)
	}
	fitted0 := NumTokensFromString(kept[0].Content)
	fitted1 := NumTokensFromString(kept[1].Content)
	if fitted0 == 0 || fitted1 == 0 {
		t.Errorf("a system message was emptied by fitting: %+v", kept)
	}
	if fitted0 >= NumTokensFromString(sys1) || fitted1 >= NumTokensFromString(sys2) {
		t.Errorf("system messages not trimmed: %+v", kept)
	}
}

func TestFit_Step3_SystemDominates(t *testing.T) {
	// System takes >80% of tokens → preserve user, trim system.
	sysContent := strings.Repeat("a ", 800)  // dominates
	userContent := strings.Repeat("b ", 100) // small, fits entirely
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}
	const budget = 500

	kept, keptIdx, count := Fit(msgs, budget)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if count > budget {
		t.Errorf("fitted total %d exceeds budget %d", count, budget)
	}
	if !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("keptIdx = %v, want [0 1]", keptIdx)
	}
	// User preserved verbatim; system trimmed.
	if NumTokensFromString(kept[1].Content) != NumTokensFromString(userContent) {
		t.Errorf("user message not preserved: %+v", kept)
	}
	if NumTokensFromString(kept[0].Content) >= NumTokensFromString(sysContent) {
		t.Errorf("system not trimmed: %+v", kept)
	}
}

func TestFit_Step3_UserDominates(t *testing.T) {
	// User takes >20% → preserve system, trim user.
	sysContent := strings.Repeat("a ", 150)  // small, fits entirely
	userContent := strings.Repeat("b ", 800) // dominates
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}
	const budget = 500

	kept, keptIdx, count := Fit(msgs, budget)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if count > budget {
		t.Errorf("fitted total %d exceeds budget %d", count, budget)
	}
	if !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("keptIdx = %v, want [0 1]", keptIdx)
	}
	// System preserved verbatim; user trimmed.
	if NumTokensFromString(kept[0].Content) != NumTokensFromString(sysContent) {
		t.Errorf("system message not preserved: %+v", kept)
	}
	if NumTokensFromString(kept[1].Content) >= NumTokensFromString(userContent) {
		t.Errorf("user not trimmed: %+v", kept)
	}
}

// TestFit_Step3_BudgetFilledByLast locks the boundary where the last message
// alone fills the budget (preserved == budget): the system share collapses to
// 0, so every retained system message is kept but trimmed to empty — it must
// NOT be reported as dropped. Python's message_fit_in retains the system entry
// with content "" in this case too.
func TestFit_Step3_BudgetFilledByLast(t *testing.T) {
	sysContent := strings.Repeat("a ", 3000) // dominates (>80% of tokens)
	userContent := strings.Repeat("b ", 600) // alone exceeds the budget
	msgs := []Message{
		{Role: "system", Content: sysContent},
		{Role: "user", Content: userContent},
	}
	const budget = 500

	kept, keptIdx, count := Fit(msgs, budget)
	if !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("keptIdx = %v, want [0 1] (both messages still kept)", keptIdx)
	}
	if count > budget {
		t.Errorf("fitted total %d exceeds budget %d", count, budget)
	}
	if kept[0].Content != "" {
		t.Errorf("system message not trimmed to empty when the last message fills the budget: %+v", kept)
	}
	if NumTokensFromString(kept[1].Content) > budget {
		t.Errorf("user message exceeds budget after trim: %+v", kept)
	}
}

func TestFit_SingleMessage(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: strings.Repeat("x ", 1000)},
	}
	kept, keptIdx, count := Fit(msgs, 100)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if !slices.Equal(keptIdx, []int{0}) {
		t.Fatalf("keptIdx = %v, want [0]", keptIdx)
	}
	if NumTokensFromString(kept[0].Content) > 100 {
		t.Errorf("single message not trimmed: %+v", kept)
	}
}

func TestFit_ZeroBudget(t *testing.T) {
	// budget <= 0 should use 8192 default and not panic.
	msgs := []Message{
		{Role: "system", Content: "hello"},
		{Role: "user", Content: "world"},
	}
	kept, keptIdx, count := Fit(msgs, 0)
	if count == 0 {
		t.Fatalf("Fit returned count 0, want > 0")
	}
	if len(kept) != 2 || !slices.Equal(keptIdx, []int{0, 1}) {
		t.Fatalf("got kept=%+v keptIdx=%v, want both messages", kept, keptIdx)
	}
	if kept[0].Content != "hello" || kept[1].Content != "world" {
		t.Errorf("messages modified with default budget: %+v", kept)
	}
}

func TestFit_Empty(t *testing.T) {
	if kept, keptIdx, count := Fit(nil, 1000); kept != nil || keptIdx != nil || count != 0 {
		t.Errorf("Fit(nil) = %v, %v, %d; want nil, nil, 0", kept, keptIdx, count)
	}
	if kept, keptIdx, count := Fit([]Message{}, 1000); len(kept) != 0 || len(keptIdx) != 0 || count != 0 {
		t.Errorf("Fit(empty) = %v, %v, %d; want 0, 0, 0", kept, keptIdx, count)
	}
}

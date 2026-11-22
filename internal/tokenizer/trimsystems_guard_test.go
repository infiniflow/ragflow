package tokenizer

import "testing"

// B2 adversarial: trimSystems is documented to recompute counts when counts
// is nil/short (useCounts=false). Line 173 currently indexes counts[i] before
// that fallback, so nil counts with >=2 system messages panics. The previous
// implementation did not panic for this input.
func TestTrimSystemsNilCountsNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("trimSystems panicked with nil counts: %v", r)
		}
	}()
	sys := []Message{
		{Role: "system", Content: "You are a careful assistant. Follow the rules precisely."},
		{Role: "system", Content: "Second system message with enough tokens to matter here."},
	}
	trimSystems(sys, 200, nil)
}

func TestTrimSystemsShortCountsNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("trimSystems panicked with short counts: %v", r)
		}
	}()
	sys := []Message{
		{Role: "system", Content: "First system message long enough to tokenize."},
		{Role: "system", Content: "Second system message long enough to tokenize."},
		{Role: "system", Content: "Third system message long enough to tokenize."},
	}
	// len(counts)=1 < len(sys)=3 -> useCounts=false; indexing counts[1] panics.
	trimSystems(sys, 200, []int{10})
}

package tokenizer

import (
	"reflect"
	"testing"
)

// The BPE batch loop defers candidates that are cheaper than the round in progress
// (the batch definition finishes the round first). A deferred candidate must never
// be LOST when the heap drains: dropping one leaves the piece split into more
// symbols than the merge table allows, i.e. an over-count — the direction that makes
// a provider reject a request.
//
// The table below is deliberately NOT training-consistent (the rank-0 merge needs a
// symbol that only the rank-5 merge creates). A well-formed table cannot reach this
// state, which is exactly why it is easy to lose: the loop used to exit with the
// candidate still deferred and segment "abc" as two tokens.
func TestBpeMergeDoesNotLoseDeferredCandidatesWhenTheHeapDrains(t *testing.T) {
	m := &bpeModel{merges: map[string]int{
		"a b":  5,
		"ab c": 0,
	}}

	got := m.merge([]string{"a", "b", "c"})
	want := []string{"abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deferred merge was dropped: got %v, want %v", got, want)
	}
}

// Sanity counterpart: with a training-consistent table the deferred path is never
// taken, so the ordinary cheapest-pair-first behaviour is unchanged.
func TestBpeMergePicksTheCheapestPairFirst(t *testing.T) {
	m := &bpeModel{merges: map[string]int{
		"a b":  0,
		"ab c": 1,
	}}

	got := m.merge([]string{"a", "b", "c"})
	want := []string{"abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

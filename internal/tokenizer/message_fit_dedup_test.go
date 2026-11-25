package tokenizer

import "testing"

// Message set that overflows budget and keeps many system messages, so the
// trimSystems path (the target of the count-reuse change) is exercised.
func fitOverflowMessages() []Message {
	msgs := make([]Message, 0, 25)
	for i := 0; i < 20; i++ {
		msgs = append(msgs, Message{Role: "system", Content: "You are a careful assistant. Follow the rules precisely and cite sources when needed. " + string(rune('a'+i%26))})
	}
	msgs = append(msgs, Message{Role: "user", Content: "Summarize the following document with detailed bullet points and references."})
	return msgs
}

func BenchmarkFitOverflowTrimSystems(b *testing.B) {
	msgs := fitOverflowMessages()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Fit(msgs, 200)
	}
}

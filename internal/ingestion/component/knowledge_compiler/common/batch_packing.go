package common

// PackBatches groups chunks into LLM-context-window-sized batches using greedy
// bin-packing by token budget. It mirrors Python's split_chunks / build_chunk_batches:
// a single chunk is NEVER split, even when it exceeds maxTokens — in that case it
// is placed alone in its own batch. Returns nil when there are no chunks.
func PackBatches(chunks []Chunk, maxTokens int, tok Tokenizer) [][]Chunk {
	if len(chunks) == 0 {
		return nil
	}
	if maxTokens <= 0 {
		return [][]Chunk{chunks}
	}
	var batches [][]Chunk
	var cur []Chunk
	curTokens := 0

	flush := func() {
		if len(cur) > 0 {
			batches = append(batches, cur)
			cur = nil
			curTokens = 0
		}
	}

	for _, c := range chunks {
		text := c.Text
		if text == "" {
			text = c.Content
		}
		t := numTokens(tok, text)
		if t > maxTokens {
			// Oversized chunk: emit current batch, then the chunk alone.
			flush()
			batches = append(batches, []Chunk{c})
			continue
		}
		if curTokens+t > maxTokens && len(cur) > 0 {
			flush()
		}
		cur = append(cur, c)
		curTokens += t
	}
	flush()
	return batches
}

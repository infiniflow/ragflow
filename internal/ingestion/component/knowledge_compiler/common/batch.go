package common

import "context"

// BatchHandler processes one batch of chunks. Returning an error aborts the
// pipeline (RunChunkedPipeline propagates it).
type BatchHandler func(ctx context.Context, batch []Chunk) error

// RunChunkedPipeline packs chunks into token-budgeted batches and invokes h for
// each, in order, honouring ctx cancellation. It is the Go equivalent of
// Python's run_chunked_pipeline engine step.
func RunChunkedPipeline(ctx context.Context, chunks []Chunk, maxTokens int, tok Tokenizer, h BatchHandler) error {
	batches := PackBatches(chunks, maxTokens, tok)
	for _, b := range batches {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := h(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

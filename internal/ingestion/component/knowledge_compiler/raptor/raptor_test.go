package raptor

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/tokenizer"
)

// fakeChat records the last request and returns scripted responses.
type fakeChat struct {
	calls   int
	lastReq common.ChatRequest
	// responses[i] is returned on the i-th call; nil entry means an error.
	responses []*common.ChatResponse
	errs      []error
}

func (f *fakeChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	f.lastReq = req
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i < len(f.responses) {
		return f.responses[i], nil
	}
	return &common.ChatResponse{Content: "ok"}, nil
}

func depsWithChat(c common.ChatInvoker) common.Deps {
	return common.Deps{Chat: c, Embed: nil, TenantID: "t"}
}

func TestSummarizeTextsStripsThinkPreamble(t *testing.T) {
	f := &fakeChat{responses: []*common.ChatResponse{{Content: "<think>let me think...\n\n</think>Final summary title\nbody"}}}
	got, err := summarizeTexts(context.Background(), depsWithChat(f), "llm", "sys", "user", 512)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
		t.Fatalf("think preamble not stripped: %q", got)
	}
	if !strings.Contains(got, "Final summary title") {
		t.Fatalf("body lost: %q", got)
	}
}

func TestSummarizeTextsStripsTruncationMarker(t *testing.T) {
	marker := strings.Repeat("·", 6) + "\n由于长度的原因，回答被截断了，要继续吗？"
	f := &fakeChat{responses: []*common.ChatResponse{{Content: "title\nbody " + marker}}}
	got, err := summarizeTexts(context.Background(), depsWithChat(f), "llm", "sys", "user", 512)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(got, "回答被截断了") {
		t.Fatalf("truncation marker not stripped: %q", got)
	}
}

func TestSummarizeTextsRetriesOnErrorMarker(t *testing.T) {
	f := &fakeChat{
		responses: []*common.ChatResponse{
			{Content: "**ERROR** something broke"},
			{Content: "title\nclean summary"},
		},
	}
	got, err := summarizeTexts(context.Background(), depsWithChat(f), "llm", "sys", "user", 512)
	if err != nil {
		t.Fatalf("unexpected err after retry: %v", err)
	}
	if got != "title\nclean summary" {
		t.Fatalf("expected clean summary after retry, got %q", got)
	}
	if f.calls != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", f.calls)
	}
}

func TestSummarizeTextsFailsAfterMaxRetries(t *testing.T) {
	f := &fakeChat{responses: []*common.ChatResponse{
		{Content: "**ERROR** 1"}, {Content: "**ERROR** 2"}, {Content: "**ERROR** 3"},
	}}
	if _, err := summarizeTexts(context.Background(), depsWithChat(f), "llm", "sys", "user", 512); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if f.calls != raptorMaxRetries {
		t.Fatalf("expected %d attempts, got %d", raptorMaxRetries, f.calls)
	}
}

func TestSummarizeTextsPassesMaxTokens(t *testing.T) {
	f := &fakeChat{responses: []*common.ChatResponse{{Content: "title\nbody"}}}
	if _, err := summarizeTexts(context.Background(), depsWithChat(f), "llm", "sys", "user", 1024); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.lastReq.MaxTokens == nil || *f.lastReq.MaxTokens != 1024 {
		t.Fatalf("MaxTokens not passed through: %v", f.lastReq.MaxTokens)
	}
}

func TestBuildClusterContentJoinsWithSingleNewline(t *testing.T) {
	// delimiter must be "\n" to match Python's "\n".join, not "\n\n".
	out := buildClusterContent([]string{"a", "b", "c"}, []int{0, 1, 2}, common.DefaultLLMContextLength, 512)
	if out != "a\nb\nc" {
		t.Fatalf("expected single-newline join, got %q", out)
	}
}

func TestBuildClusterContentTruncatesPerChunk(t *testing.T) {
	// A long text must be truncated to the per-chunk token budget so the
	// cluster fits the LLM context window (Python len_per_chunk).
	long := strings.Repeat("hello world ", 200)
	out := buildClusterContent([]string{long}, []int{0}, common.DefaultLLMContextLength, 512)
	per := (common.DefaultLLMContextLength - 512) / 1
	if tokenizer.NumTokensFromString(out) > per {
		t.Fatalf("output exceeded per-chunk budget: %d > %d", tokenizer.NumTokensFromString(out), per)
	}
}

// TestBuildTreeNoPanicWhenAllSummariesFail guards the divide-by-zero that
// occurred when every deepest cluster failed: buildClusterContent divides by
// len(idxs), and the root synthesis built a cluster from allIndices(0) when
// topLevelTexts was empty. The root is now skipped and the partial tree is
// returned without error.
func TestBuildTreeNoPanicWhenAllSummariesFail(t *testing.T) {
	errs := make([]error, 16)
	for i := range errs {
		errs[i] = context.DeadlineExceeded
	}
	f := &fakeChat{errs: errs}
	deps := common.Deps{Chat: f, Embed: nil, TenantID: "t"}
	// Pre-computed vectors so the tree never needs to call the embedder.
	chunks := []common.Chunk{
		{Text: "alpha", Vector: []float32{1, 0, 0, 0}},
		{Text: "beta", Vector: []float32{0, 1, 0, 0}},
	}
	var products []common.Product
	if err := buildTree(context.Background(), deps, "llm", "t", "d", chunks, 4, "", common.Param{}, &products); err != nil {
		t.Fatalf("buildTree returned unexpected error: %v", err)
	}
	if len(products) != 0 {
		t.Fatalf("expected no products when every summary fails, got %d", len(products))
	}
}

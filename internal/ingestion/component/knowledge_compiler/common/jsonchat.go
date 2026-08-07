package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// fencedJSONRE matches a ```json ... ``` or ``` ... ``` fenced block. Models
// frequently wrap JSON in such fences even when JSONMode is requested, which
// previously slipped through as an unparseable "_raw" payload and was silently
// dropped by the extraction parser (a data-loss path). We strip the fence and
// retry before giving up.
var fencedJSONRE = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

// GenJSON dispatches a chat call in JSON mode and parses the response into a
// map. It first tries the raw content, then a fenced ```json ... ``` block the
// model may have wrapped around the JSON, and finally the outermost {...} span
// (handles "Here is the JSON: {...}" prose). A genuine parse failure is
// returned as an error (NOT a silent {"_raw": ...}) so the caller fails loudly
// or retries rather than silently dropping the extraction — the
// highest-frequency LLM integration must not lose knowledge units on a
// formatting hiccup.
func GenJSON(ctx context.Context, chat ChatInvoker, req ChatRequest) (map[string]any, error) {
	req.JSONMode = true
	resp, err := chat.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, candidate := range jsonCandidates(resp.Content) {
		if m, ok := tryUnmarshalJSON(candidate); ok {
			return m, nil
		}
	}
	// Diagnostic: surface how far parsing got so a formatting failure is not
	// opaque. Each candidate is reported with its own unmarshal error so we can
	// tell an unfenced/truncated payload from a genuine syntax error.
	for i, candidate := range jsonCandidates(resp.Content) {
		_, err := tryUnmarshalJSONErr(candidate)
		log.Printf("knowledge_compiler: GenJSON candidate[%d] len=%d parse_err=%v body=%q", i, len(candidate), err, truncate(candidate, 300))
	}
	return nil, fmt.Errorf("knowledge_compiler: LLM response is not parseable JSON: %q", truncate(resp.Content, 200))
}

// jsonCandidates yields progressively "cleaned" versions of an LLM reply that
// may contain JSON: the raw text, a fenced ```json ... ``` block, and the
// outermost {...} span.
func jsonCandidates(s string) []string {
	cands := []string{s}
	if loc := fencedJSONRE.FindStringSubmatch(s); len(loc) == 2 {
		cands = append(cands, loc[1])
	}
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			cands = append(cands, s[i:j+1])
		}
	}
	return cands
}

func tryUnmarshalJSON(s string) (map[string]any, bool) {
	m, err := tryUnmarshalJSONErr(s)
	return m, err == nil
}

func tryUnmarshalJSONErr(s string) (map[string]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty candidate")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

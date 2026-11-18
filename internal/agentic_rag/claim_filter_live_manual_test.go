//go:build manual

//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package agentic_rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestClaimFilterLiveManual is the end-to-end check for the claim exit: real
// claim rows from a live corpus, the real decision-model API, and the rendered
// page measured before and after the filter.
//
// It spends tokens on the configured decision model, which is what puts it in the
// manual tier:
//
//	CLAIM_LIVE_TENANT=<tenant> CLAIM_LIVE_QUESTION="<question>" \
//	RAGFLOW_DECISION_BACKEND=decision-api \
//	RAGFLOW_DECISION_URL=https://api.bamboonode.dev/v1/decisions \
//	RAGFLOW_DECISION_KEY=<key> \
//	RAGFLOW_DECISION_MODEL=startlux-decision-27b-autothink \
//	  bash build.sh --test-manual -run ClaimFilterLive -v ./internal/agentic_rag/
//
// The probability quartiles are printed because the threshold is model-specific:
// the shipped 0.30 was fitted on decision-model-preview, and a claim line is far
// shorter than the passage that threshold was measured on.
func TestClaimFilterLiveManual(t *testing.T) {
	tenant := strings.TrimSpace(os.Getenv("CLAIM_LIVE_TENANT"))
	question := strings.TrimSpace(os.Getenv("CLAIM_LIVE_QUESTION"))
	if tenant == "" || question == "" {
		t.Fatal("set CLAIM_LIVE_TENANT and CLAIM_LIVE_QUESTION")
	}
	es := liveES{
		url:  liveEnv("CLAIM_LIVE_ES", "http://localhost:1200"),
		user: liveEnv("CLAIM_LIVE_ES_USER", "elastic"),
		pass: liveEnv("CLAIM_LIVE_ES_PASS", "infini_rag_flow"),
	}
	limit := 12
	index := "ragflow_" + tenant

	rows := es.search(t, index, map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{"bool": map[string]interface{}{
			"filter": []interface{}{
				map[string]interface{}{"term": map[string]interface{}{"entity_type_kwd": "claim"}},
				map[string]interface{}{"term": map[string]interface{}{"scope_kwd": "doc"}},
			},
		}},
		"_source": []string{"content_with_weight", "source_chunk_ids", "doc_id"},
	})

	claims := make([]claimHit, 0, len(rows))
	anchors := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		h, ok := parseClaimHit(row)
		if !ok {
			continue
		}
		claims = append(claims, h)
		for _, id := range h.sourceChunkIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			anchors = append(anchors, id)
		}
	}
	if len(claims) == 0 {
		t.Fatalf("no usable claim rows under index %s", index)
	}

	chunkRunes := es.chunkRunes(t, index, anchors)

	ctx := withDecisionScope(WithQuery(context.Background(), question), tenant, "")
	if !decisionAvailable(ctx) {
		t.Fatal("decision backend unavailable: set RAGFLOW_DECISION_BACKEND=decision-api and RAGFLOW_DECISION_KEY")
	}

	// Score first, so the distribution is visible even when the current threshold
	// keeps everything. filterClaims then re-uses this pass through the judge's
	// own memo, which is why the second call costs no extra requests.
	texts := make([]string, 0, len(claims))
	for _, c := range claims {
		texts = append(texts, claimJudgeText(c))
	}
	judgeStart := time.Now()
	verdicts := decisionJudge(ctx, question, texts)
	judgeElapsed := time.Since(judgeStart)
	probs := make([]float64, 0, len(verdicts))
	for _, v := range verdicts {
		if v.Judged {
			probs = append(probs, v.Probability)
		}
	}
	sort.Float64s(probs)

	before := claimEvidenceBlock(context.Background(), claims)
	start := time.Now()
	filtered := filterClaims(ctx, question, claims)
	elapsed := time.Since(start)
	after := claimEvidenceBlock(context.Background(), filtered)

	beforeRunes := utf8.RuneCountInString(before)
	afterRunes := utf8.RuneCountInString(after)

	fmt.Printf("\n=== claim exit, live ===\n")
	fmt.Printf("question           : %s\n", question)
	fmt.Printf("anchor chunks      : %d  (raw text %d runes)\n", len(anchors), chunkRunes)
	fmt.Printf("claims in          : %d\n", len(claims))
	fmt.Printf("claims out         : %d  (dropped %d, %.0f%%)\n",
		len(filtered), len(claims)-len(filtered), 100*float64(len(claims)-len(filtered))/float64(len(claims)))
	fmt.Printf("judged             : %d / %d\n", len(probs), len(verdicts))
	if len(probs) > 0 {
		fmt.Printf("P(relevant) min    : %.3f\n", probs[0])
		fmt.Printf("P(relevant) p25    : %.3f\n", probs[len(probs)/4])
		fmt.Printf("P(relevant) median : %.3f\n", probs[len(probs)/2])
		fmt.Printf("P(relevant) p75    : %.3f\n", probs[3*len(probs)/4])
		fmt.Printf("P(relevant) max    : %.3f\n", probs[len(probs)-1])
	}
	fmt.Printf("page runes before  : %d\n", beforeRunes)
	fmt.Printf("page runes after   : %d  (%.1f%% of before)\n", afterRunes, 100*float64(afterRunes)/float64(beforeRunes))
	fmt.Printf("claims vs raw text : before %.2fx, after %.2fx of the anchored chunk text\n",
		float64(beforeRunes)/float64(chunkRunes), float64(afterRunes)/float64(chunkRunes))
	fmt.Printf("judge wall time    : %s (scoring) + %s (filter pass)\n", judgeElapsed, elapsed)

	if len(filtered) > len(claims) {
		t.Fatalf("filter kept more claims than it was given: %d > %d", len(filtered), len(claims))
	}
	if afterRunes > beforeRunes {
		t.Fatalf("filtered page is larger: %d > %d", afterRunes, beforeRunes)
	}
}

func liveEnv(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// liveES is the smallest read-only client the check needs: it only ever issues
// _search, so the live corpus is never written.
type liveES struct {
	url  string
	user string
	pass string
}

func (e liveES) search(t *testing.T, index string, body map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode search body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(e.url, "/")+"/"+index+"/_search", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.user != "" {
		req.SetBasicAuth(e.user, e.pass)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("search %s: %v", index, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search %s: %s: %s", index, resp.Status, string(payload))
	}
	var out struct {
		Hits struct {
			Hits []struct {
				Score  *float64               `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	rows := make([]map[string]interface{}, 0, len(out.Hits.Hits))
	for _, h := range out.Hits.Hits {
		row := h.Source
		if row == nil {
			row = map[string]interface{}{}
		}
		if h.Score != nil {
			row["_score"] = *h.Score
		}
		rows = append(rows, row)
	}
	return rows
}

// chunkRunes is the raw text the claim page is competing with: the anchored
// chunks the same claims were compiled from.
func (e liveES) chunkRunes(t *testing.T, index string, ids []string) int {
	t.Helper()
	if len(ids) == 0 {
		return 0
	}
	rows := e.search(t, index, map[string]interface{}{
		"size":    len(ids),
		"query":   map[string]interface{}{"terms": map[string]interface{}{"_id": ids}},
		"_source": []string{"content_with_weight"},
	})
	total := 0
	for _, row := range rows {
		total += utf8.RuneCountInString(stringFromAny(row["content_with_weight"]))
	}
	return total
}

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
	"context"
	"os"
	"testing"

	"ragflow/internal/dao"
)

// TestDecisionLiveAgainstConfiguredModel drives the whole judge path against the
// rows a tenant configures in the UI: resolve tenant -> model -> instance ->
// base_url + key, then one real HTTP call per passage.
//
// It is opt-in because it spends tokens and needs a reachable endpoint. The unit
// tests cover the resolution logic with fixtures; this exists to prove the pieces
// still line up against a real deployment, which is what fixtures cannot catch —
// a renamed column, a base URL stored under a different JSON key, a suffix that
// no longer matches.
//
//	RAGFLOW_DECISION_LIVE=1 RAGFLOW_DECISION_LIVE_TENANT=<id> \
//	  go test ./internal/agentic_rag -run DecisionLive -v
func TestDecisionLiveAgainstConfiguredModel(t *testing.T) {
	if os.Getenv("RAGFLOW_DECISION_LIVE") == "" {
		t.Skip("set RAGFLOW_DECISION_LIVE=1 to run against the configured model")
	}
	tenantID := os.Getenv("RAGFLOW_DECISION_LIVE_TENANT")
	if tenantID == "" {
		t.Fatal("set RAGFLOW_DECISION_LIVE_TENANT")
	}

	orig := dao.DB
	t.Cleanup(func() { dao.DB = orig })
	if err := dao.InitDB(context.Background(), false); err != nil {
		t.Fatalf("init db: %v", err)
	}

	question := "RAG 的定义是什么？"
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	ctx = WithQuery(ctx, question)
	ctx = withDecisionScope(ctx, tenantID, "")

	b := resolveDecisionBackend(ctx)
	if !b.available() {
		t.Fatalf("no decision backend resolved for tenant %s", tenantID)
	}
	t.Logf("resolved backend: model=%s url=%s key_len=%d", b.model, b.url, len(b.key))

	verdicts := b.judge(ctx, question, []string{
		"RAG 全称 Retrieval-Augmented Generation，中文叫检索增强生成。它先用检索器从知识库里找出与问题相关的片段。",
		"今天天气很好，适合出门散步。",
	})
	if len(verdicts) != 2 {
		t.Fatalf("verdicts=%d, want 2", len(verdicts))
	}
	for i, v := range verdicts {
		t.Logf("passage %d: useful=%v judged=%v prob=%.3f", i, v.Useful, v.Judged, v.Probability)
	}
	if !verdicts[0].Judged {
		t.Error("the on-topic passage was not judged — the endpoint answered nothing")
	}
}

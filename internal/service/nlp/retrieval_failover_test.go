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

package nlp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

func TestRetrievalFailoverTarget(t *testing.T) {
	embdGroup := "gp-4e23f42141c345aab3c7fd6f8a0de8fc"
	rerankGroup := "gp-1f8e2c9a55b64d0fb3a72d7c4e9a10bb"

	tests := []struct {
		name      string
		embdRef   string
		rerankRef string
		wantType  entity.ModelType
		wantRef   string
	}{
		{"rerank-only group", "bge-m3@BAAI", rerankGroup, entity.ModelTypeRerank, rerankGroup},
		{"embedding-only group", embdGroup, "bge-reranker-v2@BAAI", entity.ModelTypeEmbedding, embdGroup},
		{"no groups", "bge-m3@BAAI", "bge-reranker-v2@BAAI", entity.ModelType(0), ""},
		{"both groups stay uncovered", embdGroup, rerankGroup, entity.ModelType(0), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failoverType, failoverRef := RetrievalFailoverTarget(tt.embdRef, tt.rerankRef)
			if failoverType != tt.wantType || failoverRef != tt.wantRef {
				t.Fatalf("RetrievalFailoverTarget(%q, %q) = (%v, %q), want (%v, %q)",
					tt.embdRef, tt.rerankRef, failoverType, failoverRef, tt.wantType, tt.wantRef)
			}
		})
	}
}

func TestRetrievalFailoverCanRetry(t *testing.T) {
	rerankRetry := RetrievalFailoverCanRetry(entity.ModelTypeRerank)
	embeddingRetry := RetrievalFailoverCanRetry(entity.ModelTypeEmbedding)
	noneRetry := RetrievalFailoverCanRetry(entity.ModelType(0))

	if !reretryMatches(rerankRetry, ErrRerankModelSide) {
		t.Fatal("rerank failover must retry on ErrRerankModelSide")
	}
	if reretryMatches(rerankRetry, ErrEmbeddingModelSide) {
		t.Fatal("rerank failover must not retry on ErrEmbeddingModelSide")
	}
	if !reretryMatches(embeddingRetry, ErrEmbeddingModelSide) {
		t.Fatal("embedding failover must retry on ErrEmbeddingModelSide")
	}
	if embeddingRetry(errors.New("engine down")) {
		t.Fatal("embedding failover must not retry on engine errors")
	}
	if reretryMatches(rerankRetry, errors.New("engine down")) {
		t.Fatal("rerank failover must not retry on engine errors")
	}
	if noneRetry(errors.New("anything")) {
		t.Fatal("no failover target must never retry")
	}

	// Wrapped errors keep the sentinel attribution.
	wrapped := errors.Join(errors.New("score failed"), ErrRerankModelSide)
	if !reretryMatches(rerankRetry, wrapped) {
		t.Fatal("wrapped ErrRerankModelSide must still retry")
	}

	// A client request rejection (payload refused, HTTP 4xx) fails
	// identically on any member and must not flip one.
	rejected := fmt.Errorf("rerank failed: %w", &models.APIStatusError{Status: http.StatusRequestEntityTooLarge})
	if reretryMatches(rerankRetry, rejected) {
		t.Fatal("request-level 4xx must not flip a member")
	}
	if reretryMatches(embeddingRetry, rejected) {
		t.Fatal("request-level 4xx must not flip a member (embedding side)")
	}
	memberDown := fmt.Errorf("%w: rerank failed: %w", ErrRerankModelSide, &models.APIStatusError{Status: http.StatusBadGateway})
	if !reretryMatches(rerankRetry, memberDown) {
		t.Fatal("5xx member failure must still flip")
	}
}

func reretryMatches(canRetry func(error) bool, target error) bool {
	return canRetry(target)
}

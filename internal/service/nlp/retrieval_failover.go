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

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

// RetrievalFailoverTarget resolves which retrieval-side model reference
// carries gp- group failover: exactly one of the embedding/rerank references
// may be a group. With both behind groups the failing side cannot be
// attributed to one group from the error alone, so failover covers the
// single-group case. Returns a zero model type when neither reference is a
// group reference.
func RetrievalFailoverTarget(embdRef, rerankRef string) (entity.ModelType, string) {
	_, embdIsGroup := common.ParseGroupModelRef(embdRef)
	_, rerankIsGroup := common.ParseGroupModelRef(rerankRef)
	switch {
	case embdIsGroup && !rerankIsGroup:
		return entity.ModelTypeEmbedding, embdRef
	case rerankIsGroup && !embdIsGroup:
		return entity.ModelTypeRerank, rerankRef
	}
	return entity.ModelType(0), ""
}

// RetrievalFailoverCanRetry attributes an invocation failure to the model
// behind the failover group: document engine outages and request errors say
// nothing about the member's health, so they must not deactivate a healthy
// member. Only the sentinel-wrapped model-side failures may flip, and
// client-side request rejections (HTTP 4xx payload refusals) never do —
// replaying the same request on another member would fail identically.
func RetrievalFailoverCanRetry(failoverType entity.ModelType) func(error) bool {
	return func(err error) bool {
		if models.IsRequestRejected(err) {
			return false
		}
		switch failoverType {
		case entity.ModelTypeEmbedding:
			return errors.Is(err, ErrEmbeddingModelSide)
		case entity.ModelTypeRerank:
			return errors.Is(err, ErrRerankModelSide)
		}
		return false
	}
}

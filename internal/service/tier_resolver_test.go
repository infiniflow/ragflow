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

//go:build !ee

package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/permission"
)

// TestDefaultTierResolverCommunityReturnsDefault: the community (!ee) build has
// no billing tiers, so its default resolver must route every tenant to the
// default lane, which reuses the OSS community queue. That makes an OSS split a
// no-op rename and lets OSS and EE share one NATS server. The ee build's
// resolver is tested separately (tier_resolver_ee_test.go) and must not be
// affected by this change.
func TestDefaultTierResolverCommunityReturnsDefault(t *testing.T) {
	got, err := defaultTierResolver(t.Context(), "any-tenant")
	if err != nil {
		t.Fatalf("defaultTierResolver error: %v", err)
	}
	if got != common.TierDefault {
		t.Fatalf("community defaultTierResolver = %q, want TierDefault", got)
	}
}

// TestIngestionCommunityDefaultPublishesDefaultTier (end-to-end): with no
// resolver injected, the community (!ee) IngestionTaskService must resolve the
// default resolver and publish to the default lane, which equals the OSS
// community subject "tasks.RAGFLOW". This closes the loop between the community
// resolver and the producer routing; ee (with a billing resolver) is covered by
// ingestion_task_tier_test.go via an injected resolver.
func TestIngestionCommunityDefaultPublishesDefaultTier(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	if _, err := svc.CreateForDocuments(permission.WithIdentity(t.Context(), "user-1", ""), "kb-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments: %v", err)
	}
	if publisher.subject != common.TaskSubject {
		t.Fatalf("community default published to %q, want %q (default == OSS community subject)", publisher.subject, common.TaskSubject)
	}
}

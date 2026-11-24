//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"strings"
	"testing"
)

func TestAzureDevOpsCommitFingerprintFitsDocumentColumn(t *testing.T) {
	commit := strings.Repeat("a", 40)
	item := azureDevOpsItem{LatestProcessedChange: &azureDevOpsChange{CommitID: commit}}
	connector, err := NewAzureDevOpsConnector(map[string]any{"organization": "contoso", "credentials": map[string]any{"azure_devops_pat": "token"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := connector.buildAzureDevOpsCodeDocument(azureDevOpsRepository{Project: "p", Name: "r", Branch: "main"}, item, []byte("content"))
	if doc.Metadata["commit_id"] != commit {
		t.Fatal("metadata must preserve the full commit ID")
	}
	if len(doc.Fingerprint) != 32 {
		t.Fatalf("fingerprint length=%d must fit content_hash size=32", len(doc.Fingerprint))
	}
	if includeAzureDevOpsItem(SyncRequest{Fingerprints: map[string]string{"source": doc.Fingerprint}}, "source", item) {
		t.Fatal("unchanged item must be skipped")
	}
	item.LatestProcessedChange.CommitID = strings.Repeat("b", 40)
	if !includeAzureDevOpsItem(SyncRequest{Fingerprints: map[string]string{"source": doc.Fingerprint}}, "source", item) {
		t.Fatal("changed commit must be included")
	}
}

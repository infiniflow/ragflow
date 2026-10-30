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

package chunk

import (
	"ragflow/internal/dao"
	"ragflow/internal/service"
	"reflect"
	"testing"
)

func TestSDKChunkListSchemaAndEmptyDeletion(t *testing.T) {
	db := setupChunkTestDB(t)
	pushChunkTestDB(t, db)
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-1", "kb-1")
	engine := &listChunksSearchEngine{chunks: []map[string]interface{}{{"id": "chunk-1", "content_with_weight": "body", "important_kwd": []string{"key"}, "question_kwd": []string{"question"}}}}
	engine.chunks = append(engine.chunks, map[string]interface{}{"id": "chunk-2", "content_with_weight": "plain"})
	svc := &ChunkService{docEngine: engine, kbDAO: dao.NewKnowledgebaseDAO()}
	ctx := chunkTestContext(t, "user-1", "tenant-1")
	resp, err := svc.List(ctx, &service.ListChunksRequest{DatasetID: "kb-1", DocID: "doc-1"}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "content", "important_keywords", "questions"} {
		if _, ok := resp.Chunks[0][field]; !ok {
			t.Errorf("missing public field %s: %#v", field, resp.Chunks[0])
		}
	}
	for _, field := range []string{"important_keywords", "questions"} {
		if !reflect.DeepEqual(resp.Chunks[1][field], []interface{}{}) {
			t.Errorf("missing extraction list %s=%#v", field, resp.Chunks[1][field])
		}
	}
	for _, ids := range [][]string{nil, {}} {
		n, err := svc.RemoveChunks(ctx, &service.RemoveChunksRequest{DatasetID: "kb-1", DocID: "doc-1", ChunkIDs: ids}, "user-1")
		if err != nil || n != 0 {
			t.Errorf("empty selection: count=%d err=%v", n, err)
		}
	}
	if engine.deleteChunksCalls != 0 {
		t.Fatal("empty deletion reached engine")
	}
}

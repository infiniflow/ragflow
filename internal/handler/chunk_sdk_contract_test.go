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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/service"
	"strings"
	"testing"
)

func TestChunkUpdateRejectsMalformedSDKFields(t *testing.T) {
	for _, body := range []string{`{"content":null}`, `{"content":1}`, `{"important_keywords":[1]}`, `{"important_keywords":null}`, `{"questions":"abc"}`, `{"questions":[false]}`, `{"available":1}`, `{"available":"true"}`} {
		t.Run(body, func(t *testing.T) {
			mock := &mockChunkSvc{updateChunkFn: func(context.Context, *service.UpdateChunkRequest, string) error {
				t.Error("invalid update reached service")
				return nil
			}}
			r, h := setupChunkHandlerWithUser("user-1", mock)
			r.PATCH("/datasets/:dataset_id/documents/:document_id/chunks/:chunk_id", h.UpdateChunk)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/datasets/kb/documents/doc/chunks/chunk", strings.NewReader(body)))
			var payload struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code == 0 {
				t.Fatal("invalid update accepted")
			}
		})
	}
}

func TestChunkListAcceptsSDKID(t *testing.T) {
	mock := &mockChunkSvc{listFn: func(_ context.Context, req *service.ListChunksRequest, _ string) (*service.ListChunksResponse, error) {
		if len(req.ChunkIDs) != 1 || req.ChunkIDs[0] != "sdk-id" {
			t.Errorf("filter=%v", req.ChunkIDs)
		}
		return &service.ListChunksResponse{}, nil
	}}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.GET("/datasets/:dataset_id/documents/:document_id/chunks", h.ListChunks)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/datasets/kb/documents/doc/chunks?id=sdk-id", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}

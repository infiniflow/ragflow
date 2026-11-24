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

package filesystem

import (
	stdctx "context"
	"io"
	"net/url"
	"testing"
)

type datasetClient struct {
	paths   []string
	headers []map[string]string
}

func (f *datasetClient) Request(method, path, authKind string, headers map[string]string, body map[string]interface{}) (*HTTPResponse, error) {
	f.paths = append(f.paths, path)
	f.headers = append(f.headers, headers)
	response := `{"code":0,"data":{"docs":[]}}`
	if path == "/datasets" {
		response = `{"code":0,"data":[{"id":"kb1","name":"test"}]}`
	}
	return &HTTPResponse{StatusCode: 200, Body: []byte(response)}, nil
}
func (f *datasetClient) UploadMultipart(path, contentType string, body io.Reader) error { return nil }
func TestDatasetDocumentPaginationUsesQueryNotHeaders(t *testing.T) {
	client := &datasetClient{}
	p := NewDatasetProvider(client)
	_, err := p.listDocuments(stdctx.Background(), "test", &ListOptions{Limit: 10, Offset: 20})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(client.paths[len(client.paths)-1])
	if u.Query().Get("page") != "3" || u.Query().Get("page_size") != "10" {
		t.Fatalf("pagination missing from URL: %s", u)
	}
	if len(client.headers[len(client.headers)-1]) != 0 {
		t.Fatal("pagination must not be sent as HTTP headers")
	}
}
func TestDatasetDocumentOffsetWithoutLimitDoesNotPanic(t *testing.T) {
	client := &datasetClient{}
	p := NewDatasetProvider(client)
	_, err := p.listDocuments(stdctx.Background(), "test", &ListOptions{Offset: 100})
	if err != nil {
		t.Fatal(err)
	}
}

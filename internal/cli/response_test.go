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

package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestListResponsePrintOutHandlesMalformedData(t *testing.T) {
	cases := []struct {
		name     string
		totalKey string
		listKey  string
		new      func(map[string]interface{}) ResponseIf
	}{
		{
			name: "documents", totalKey: "total", listKey: "docs",
			new: func(data map[string]interface{}) ResponseIf { return &ListDocumentsResponse{Data: data} },
		},
		{
			name: "sync logs", totalKey: "total", listKey: "logs",
			new: func(data map[string]interface{}) ResponseIf { return &ListSyncLogsResponse{Data: data} },
		},
		{
			name: "agents", totalKey: "total", listKey: "canvas",
			new: func(data map[string]interface{}) ResponseIf { return &ListAgentsResponse{Data: data} },
		},
		{
			name: "chats", totalKey: "total", listKey: "chats",
			new: func(data map[string]interface{}) ResponseIf { return &ListChatsResponse{Data: data} },
		},
		{
			name: "searches", totalKey: "total", listKey: "search_apps",
			new: func(data map[string]interface{}) ResponseIf { return &ListSearchesResponse{Data: data} },
		},
		{
			name: "memories", totalKey: "total_count", listKey: "memory_list",
			new: func(data map[string]interface{}) ResponseIf { return &ListMemoriesResponse{Data: data} },
		},
	}
	malformedData := []struct {
		name string
		make func(totalKey, listKey string) map[string]interface{}
	}{
		{
			name: "missing total",
			make: func(_, listKey string) map[string]interface{} {
				return map[string]interface{}{listKey: []interface{}{}}
			},
		},
		{
			name: "total is not numeric",
			make: func(totalKey, listKey string) map[string]interface{} {
				return map[string]interface{}{totalKey: "many", listKey: []interface{}{}}
			},
		},
		{
			name: "list is not an array",
			make: func(totalKey, listKey string) map[string]interface{} {
				return map[string]interface{}{totalKey: float64(1), listKey: "not a list"}
			},
		},
		{
			name: "list item is not an object",
			make: func(totalKey, listKey string) map[string]interface{} {
				return map[string]interface{}{totalKey: float64(1), listKey: []interface{}{"not an object"}}
			},
		},
	}

	for _, responseCase := range cases {
		for _, dataCase := range malformedData {
			t.Run(responseCase.name+"/"+dataCase.name, func(t *testing.T) {
				response := responseCase.new(dataCase.make(responseCase.totalKey, responseCase.listKey))
				output, panicValue := capturePrintOut(t, response.PrintOut)
				if panicValue != nil {
					t.Fatalf("PrintOut panicked: %v", panicValue)
				}
				if !strings.Contains(output, "ERROR") {
					t.Fatalf("PrintOut output = %q, want a malformed-response error", output)
				}
			})
		}
	}
}

func TestListResponsePrintOutDisplaysValidData(t *testing.T) {
	cases := []struct {
		name     string
		totalKey string
		listKey  string
		new      func(map[string]interface{}) ResponseIf
	}{
		{
			name: "documents", totalKey: "total", listKey: "docs",
			new: func(data map[string]interface{}) ResponseIf { return &ListDocumentsResponse{Data: data} },
		},
		{
			name: "sync logs", totalKey: "total", listKey: "logs",
			new: func(data map[string]interface{}) ResponseIf { return &ListSyncLogsResponse{Data: data} },
		},
		{
			name: "agents", totalKey: "total", listKey: "canvas",
			new: func(data map[string]interface{}) ResponseIf { return &ListAgentsResponse{Data: data} },
		},
		{
			name: "chats", totalKey: "total", listKey: "chats",
			new: func(data map[string]interface{}) ResponseIf { return &ListChatsResponse{Data: data} },
		},
		{
			name: "searches", totalKey: "total", listKey: "search_apps",
			new: func(data map[string]interface{}) ResponseIf { return &ListSearchesResponse{Data: data} },
		},
		{
			name: "memories", totalKey: "total_count", listKey: "memory_list",
			new: func(data map[string]interface{}) ResponseIf { return &ListMemoriesResponse{Data: data} },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]interface{}{
				tc.totalKey: float64(1),
				tc.listKey:  []interface{}{map[string]interface{}{"id": "item-1"}},
			}
			output, panicValue := capturePrintOut(t, tc.new(data).PrintOut)
			if panicValue != nil {
				t.Fatalf("PrintOut panicked: %v", panicValue)
			}
			if !strings.Contains(output, "Total: 1") || !strings.Contains(output, "item-1") {
				t.Fatalf("PrintOut output = %q, want total and row", output)
			}
		})
	}
}

func TestTaskResponsePrintOutHandlesMalformedSegments(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing segments", data: map[string]interface{}{}},
		{name: "segments is not an array", data: map[string]interface{}{"segments": "bad"}},
		{name: "segment is not an object", data: map[string]interface{}{"segments": []interface{}{"bad"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, panicValue := capturePrintOut(t, (&TaskResponse{Data: tc.data}).PrintOut)
			if panicValue != nil {
				t.Fatalf("PrintOut panicked: %v", panicValue)
			}
			if !strings.Contains(output, "ERROR") {
				t.Fatalf("PrintOut output = %q, want malformed-response error", output)
			}
		})
	}
}

func capturePrintOut(t *testing.T, print func()) (string, any) {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		print()
	}()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = original
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(output), panicValue
}

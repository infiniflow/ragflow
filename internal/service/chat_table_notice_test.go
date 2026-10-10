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

package service

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestTableDatasetsWithoutFieldMap(t *testing.T) {
	withMap := entity.JSONMap{"field_map": map[string]interface{}{"country_kwd": "Country"}}
	kbs := []*entity.Knowledgebase{
		nil,
		{ID: "k1", Name: "sales", ParserID: "table"},
		{ID: "k2", Name: "typed", ParserID: "table", ParserConfig: withMap},
		{ID: "k3", Name: "empty-map", ParserID: "table", ParserConfig: entity.JSONMap{"field_map": map[string]interface{}{}}},
		{ID: "k4", Name: "docs", ParserID: "general"},
		{ID: "k5", ParserID: "table"},
	}
	got := tableDatasetsWithoutFieldMap(kbs)
	want := []string{"sales", "empty-map", "k5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAppendSQLUnavailableNotice(t *testing.T) {
	if got := appendSQLUnavailableNotice("answer", nil); got != "answer" {
		t.Fatalf("no datasets must leave the answer unchanged, got %q", got)
	}
	if got := appendSQLUnavailableNotice("", []string{"sales"}); got != "" {
		t.Fatalf("empty answer must stay empty, got %q", got)
	}
	got := appendSQLUnavailableNotice("answer", []string{"sales", "ops"})
	if !strings.HasPrefix(got, "answer\n\n") || !strings.Contains(got, "sales, ops") || !strings.Contains(got, "vector search") {
		t.Fatalf("unexpected notice: %q", got)
	}
}

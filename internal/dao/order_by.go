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

package dao

import "strings"

func pickOrderByExpr(allowed map[string]string, requested, defaultExpr string) string {
	k := strings.TrimSpace(strings.ToLower(requested))
	if k == "" {
		return defaultExpr
	}
	if expr, ok := allowed[k]; ok {
		return expr
	}
	return defaultExpr
}

var fileListOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"size":        "size",
	"type":        "type",
	"id":          "id",
}

func sanitizeFileListOrderBy(requested string) string {
	return pickOrderByExpr(fileListOrderByAllowed, requested, "create_time")
}

// dialog table (Chat model); qualify columns for JOINs with user.
var dialogListOrderByAllowed = map[string]string{
	"create_time": "dialog.create_time",
	"update_time": "dialog.update_time",
	"name":        "dialog.name",
	"id":          "dialog.id",
}

func sanitizeDialogListOrderBy(requested string) string {
	return pickOrderByExpr(dialogListOrderByAllowed, requested, "dialog.create_time")
}

var searchListOrderByAllowed = map[string]string{
	"create_time": "search.create_time",
	"update_time": "search.update_time",
	"name":        "search.name",
	"id":          "search.id",
}

func sanitizeSearchListOrderBy(requested string) string {
	return pickOrderByExpr(searchListOrderByAllowed, requested, "search.create_time")
}

// Column name only; prefixed with knowledgebase. by caller.
var knowledgebaseJoinedOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"doc_num":     "doc_num",
	"token_num":   "token_num",
	"chunk_num":   "chunk_num",
	"id":          "id",
}

func sanitizeKnowledgebaseJoinedOrderBy(requested string) string {
	return pickOrderByExpr(knowledgebaseJoinedOrderByAllowed, requested, "create_time")
}

var knowledgebasePlainOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"doc_num":     "doc_num",
	"token_num":   "token_num",
	"chunk_num":   "chunk_num",
	"id":          "id",
}

func sanitizeKnowledgebasePlainOrderBy(requested string) string {
	return pickOrderByExpr(knowledgebasePlainOrderByAllowed, requested, "create_time")
}

var userCanvasOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"title":       "title",
	"id":          "id",
}

func sanitizeUserCanvasOrderBy(requested string) string {
	return pickOrderByExpr(userCanvasOrderByAllowed, requested, "create_time")
}

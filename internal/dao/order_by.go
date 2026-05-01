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

// order_by.go contains helpers that map user-facing sort keys to fixed SQL column
// expressions for ORDER BY clauses. This avoids passing raw request strings into SQL.
package dao

import "strings"

// pickOrderByExpr resolves a client-provided sort key to a safe SQL column expression.
// It trims and lowercases requested, looks it up in allowed, and returns the matching
// expression; if requested is empty or not in allowed, it returns defaultExpr.
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

// fileListOrderByAllowed maps API sort parameter values to unqualified column names on file.
var fileListOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"size":        "size",
	"type":        "type",
	"id":          "id",
}

// sanitizeFileListOrderBy returns a safe ORDER BY column for file list queries.
// Unknown keys fall back to create_time.
func sanitizeFileListOrderBy(requested string) string {
	return pickOrderByExpr(fileListOrderByAllowed, requested, "create_time")
}

// dialogListOrderByAllowed maps API sort keys to qualified dialog columns (Chat uses dialog table).
var dialogListOrderByAllowed = map[string]string{
	"create_time": "dialog.create_time",
	"update_time": "dialog.update_time",
	"name":        "dialog.name",
	"id":          "dialog.id",
}

// sanitizeDialogListOrderBy returns a safe ORDER BY expression for dialog (chat) list queries
// when joined with user. Unknown keys fall back to dialog.create_time.
func sanitizeDialogListOrderBy(requested string) string {
	return pickOrderByExpr(dialogListOrderByAllowed, requested, "dialog.create_time")
}

// searchListOrderByAllowed maps API sort keys to qualified search table columns.
var searchListOrderByAllowed = map[string]string{
	"create_time": "search.create_time",
	"update_time": "search.update_time",
	"name":        "search.name",
	"id":          "search.id",
}

// sanitizeSearchListOrderBy returns a safe ORDER BY expression for search list queries
// when joined with user. Unknown keys fall back to search.create_time.
func sanitizeSearchListOrderBy(requested string) string {
	return pickOrderByExpr(searchListOrderByAllowed, requested, "search.create_time")
}

// knowledgebaseJoinedOrderByAllowed maps sort keys to knowledgebase column names only;
// callers prefix with "knowledgebase." in joined queries.
var knowledgebaseJoinedOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"doc_num":     "doc_num",
	"token_num":   "token_num",
	"chunk_num":   "chunk_num",
	"id":          "id",
}

// sanitizeKnowledgebaseJoinedOrderBy returns a safe column name suffix for ORDER BY on
// knowledgebase in queries that already qualify the table as knowledgebase.
func sanitizeKnowledgebaseJoinedOrderBy(requested string) string {
	return pickOrderByExpr(knowledgebaseJoinedOrderByAllowed, requested, "create_time")
}

// knowledgebasePlainOrderByAllowed maps sort keys to unqualified knowledgebase columns
// for queries without table alias ambiguity.
var knowledgebasePlainOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"name":        "name",
	"doc_num":     "doc_num",
	"token_num":   "token_num",
	"chunk_num":   "chunk_num",
	"id":          "id",
}

// sanitizeKnowledgebasePlainOrderBy returns a safe ORDER BY column for knowledgebase-only queries.
func sanitizeKnowledgebasePlainOrderBy(requested string) string {
	return pickOrderByExpr(knowledgebasePlainOrderByAllowed, requested, "create_time")
}

// userCanvasOrderByAllowed maps API sort keys to user_canvas table columns.
var userCanvasOrderByAllowed = map[string]string{
	"create_time": "create_time",
	"update_time": "update_time",
	"title":       "title",
	"id":          "id",
}

// sanitizeUserCanvasOrderBy returns a safe ORDER BY column for user_canvas list queries.
func sanitizeUserCanvasOrderBy(requested string) string {
	return pickOrderByExpr(userCanvasOrderByAllowed, requested, "create_time")
}

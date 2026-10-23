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

package permission

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QueryScope applies an authorization predicate to a database query. It is
// intended for resource listings so authorization, counting, and pagination
// happen in the database query rather than after materializing every resource
// ID.
//
// The zero value denies all rows.
type QueryScope struct {
	apply func(*gorm.DB) *gorm.DB
}

// Apply adds this authorization predicate to query. A zero-value scope fails
// closed by adding a predicate that matches no rows.
func (s QueryScope) Apply(query *gorm.DB) *gorm.DB {
	if query == nil {
		return nil
	}
	if s.apply == nil {
		return query.Where("1 = 0")
	}
	return s.apply(query)
}

func queryScopeForIDs(kind ResourceKind, ids []string) QueryScope {
	table, ok := resourceTable(kind)
	if !ok {
		return QueryScope{}
	}
	resourceIDs := append([]string(nil), ids...)
	return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
		if len(resourceIDs) == 0 {
			return query.Where("1 = 0")
		}
		values := make([]interface{}, len(resourceIDs))
		for i, id := range resourceIDs {
			values[i] = id
		}
		return query.Where(clause.IN{
			Column: clause.Column{Table: table, Name: "id"},
			Values: values,
		})
	}}
}

func resourceTable(kind ResourceKind) (string, bool) {
	switch kind {
	case ResourceKindDataset:
		return "knowledgebase", true
	case ResourceKindDocument:
		return "document", true
	case ResourceKindCanvas:
		return "user_canvas", true
	case ResourceKindAgentSession:
		return "api_4_conversation", true
	case ResourceKindMemory:
		return "memory", true
	case ResourceKindFile, ResourceKindFolder:
		return "file", true
	case ResourceKindSearchApp:
		return "search", true
	case ResourceKindChat:
		return "dialog", true
	case ResourceKindChatSession:
		return "conversation", true
	case ResourceKindChatChannel:
		return "chat_channel", true
	case ResourceKindConnector:
		return "connector", true
	case ResourceKindSyncTask:
		return "sync_logs", true
	case ResourceKindModelProvider:
		return "tenant_model_provider", true
	case ResourceKindModelInstance:
		return "tenant_model_instance", true
	case ResourceKindModel:
		return "tenant_model", true
	case ResourceKindMCPServer:
		return "mcp_server", true
	case ResourceKindSkillSpace:
		return "skill_spaces", true
	default:
		return "", false
	}
}

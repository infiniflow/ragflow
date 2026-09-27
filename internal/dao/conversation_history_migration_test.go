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

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupConversationHistoryMigrationDB builds a schema that mirrors a database
// created before the split: the Go entities drop the message and reference
// columns, so add them back the way the previous schema had them.
func setupConversationHistoryMigrationDB(t *testing.T, legacyColumns bool) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	models := []interface{}{
		&entity.ChatSession{},
		&entity.ConversationMessage{},
		&entity.ConversationReference{},
		&entity.API4Conversation{},
		&entity.API4ConversationMessage{},
		&entity.API4ConversationReference{},
		&entity.SystemSettings{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if legacyColumns {
		for _, table := range []string{"conversation", "api_4_conversation"} {
			for _, column := range []string{"message", "reference"} {
				if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s longtext", table, column)).Error; err != nil {
					t.Fatalf("failed to add %s.%s: %v", table, column, err)
				}
			}
		}
	}

	return db
}

func insertLegacyConversation(t *testing.T, db *gorm.DB, table, id string, message, reference string) {
	t.Helper()
	if err := db.Exec(fmt.Sprintf("INSERT INTO %s (id, dialog_id, user_id, message, reference) VALUES (?, ?, ?, ?, ?)", table),
		id, "dialog-"+id, "user-"+id, message, reference).Error; err != nil {
		t.Fatalf("failed to insert %s %s: %v", table, id, err)
	}
}

// insertConversation adds a parent row with no payload columns at all, the way
// the Go schema creates them.
func insertConversation(t *testing.T, db *gorm.DB, table, id string) {
	t.Helper()
	if err := db.Exec(fmt.Sprintf("INSERT INTO %s (id, dialog_id, user_id) VALUES (?, ?, ?)", table),
		id, "dialog-"+id, "user-"+id).Error; err != nil {
		t.Fatalf("failed to insert %s %s: %v", table, id, err)
	}
}

func countHistoryRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("failed to count %s: %v", table, err)
	}
	return count
}

func databaseVersion(t *testing.T, db *gorm.DB) string {
	t.Helper()
	version, err := GetDatabaseMigrationVersion(t.Context(), db)
	if err != nil {
		t.Fatalf("failed to read database version: %v", err)
	}
	return version
}

func writeDatabaseVersion(t *testing.T, db *gorm.DB, version string) {
	t.Helper()
	if err := setDatabaseMigrationVersion(t.Context(), db, version); err != nil {
		t.Fatalf("failed to write database version %s: %v", version, err)
	}
}

func TestMigrateConversationHistorySplitsPayloads(t *testing.T) {
	message := `[{"role":"user","content":"hi","id":"m1"},{"role":"assistant","content":"<think>reasoning</think>hello","id":"m2","reference":[{"chunk-0":{"content":"chunk"}}]}]`
	reference := `[{"chunks":["a"],"doc_aggs":[]},{"chunks":["b"],"doc_aggs":[]}]`
	db := setupConversationHistoryMigrationDB(t, true)
	insertLegacyConversation(t, db, "conversation", "c1", message, reference)
	insertLegacyConversation(t, db, "api_4_conversation", "a1", message, reference)

	if err := migrateConversationHistory(t.Context(), db); err != nil {
		t.Fatalf("migrateConversationHistory: %v", err)
	}

	for _, tc := range []struct{ messageTable, referenceTable string }{
		{conversationMessageTable, conversationReferenceTable},
		{apiConversationMessageTable, apiConversationReferenceTable},
	} {
		if got := countHistoryRows(t, db, tc.messageTable); got != 2 {
			t.Fatalf("%s: expected 2 messages, got %d", tc.messageTable, got)
		}
		if got := countHistoryRows(t, db, tc.referenceTable); got != 2 {
			t.Fatalf("%s: expected 2 references, got %d", tc.referenceTable, got)
		}
	}

	for _, tc := range []struct{ table, conversationID string }{
		{conversationMessageTable, "c1"},
		{apiConversationMessageTable, "a1"},
	} {
		var msg entity.ConversationMessage
		if err := db.Table(tc.table).Where("conversation_id = ? AND position = ?", tc.conversationID, 1).Take(&msg).Error; err != nil {
			t.Fatalf("load message from %s: %v", tc.table, err)
		}
		if derefString(msg.Role) != "assistant" || derefString(msg.Content) != "<think>reasoning</think>hello" || derefString(msg.MessageID) != "m2" {
			t.Fatalf("unexpected assistant message in %s: %+v", tc.table, msg)
		}
	}

	var storedReference string
	err := db.Table(conversationReferenceTable).Select("reference").
		Where("conversation_id = ? AND position = ?", "c1", 1).Row().Scan(&storedReference)
	if err != nil {
		t.Fatalf("load reference: %v", err)
	}
	if storedReference != `{"chunks":["b"],"doc_aggs":[]}` {
		t.Fatalf("unexpected reference: %s", storedReference)
	}

	// Rows backfilled from a payload carry no explicit message binding.
	var messagePos *int
	err = db.Table(conversationReferenceTable).Select("message_position").
		Where("conversation_id = ? AND position = ?", "c1", 1).Row().Scan(&messagePos)
	if err != nil {
		t.Fatalf("load message_position: %v", err)
	}
	if messagePos != nil {
		t.Fatalf("expected nil message_position, got %d", *messagePos)
	}

	if got := databaseVersion(t, db); got != conversationHistoryTargetVersion {
		t.Fatalf("expected database version %s, got %s", conversationHistoryTargetVersion, got)
	}
}

func TestMigrateConversationHistoryIsIdempotent(t *testing.T) {
	message := `[{"role":"user","content":"hi","id":"m1"},{"role":"assistant","content":"hello","id":"m2"}]`
	db := setupConversationHistoryMigrationDB(t, true)
	insertLegacyConversation(t, db, "conversation", "c1", message, `[{"chunks":["a"]}]`)
	insertLegacyConversation(t, db, "conversation", "c2", `[]`, `[]`)

	for range 2 {
		// Rewind the marker so the second pass really reruns the backfill
		// instead of short-circuiting on the recorded version.
		writeDatabaseVersion(t, db, modelMigrationTargetVersion)
		if err := migrateConversationHistory(t.Context(), db); err != nil {
			t.Fatalf("migrateConversationHistory: %v", err)
		}
	}

	if got := countHistoryRows(t, db, conversationMessageTable); got != 2 {
		t.Fatalf("expected 2 messages after rerun, got %d", got)
	}
	if got := countHistoryRows(t, db, conversationReferenceTable); got != 1 {
		t.Fatalf("expected 1 reference after rerun, got %d", got)
	}
}

func TestMigrateConversationHistoryKeepsRowsWrittenAfterSplit(t *testing.T) {
	db := setupConversationHistoryMigrationDB(t, true)
	insertLegacyConversation(t, db, "conversation", "c1", `[{"role":"user","content":"hi","id":"m1"}]`, `[]`)
	if err := db.Table(conversationMessageTable).Create(map[string]interface{}{
		"conversation_id": "c1", "position": 0, "role": "assistant", "content": "written by the runtime",
	}).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}

	if err := migrateConversationHistory(t.Context(), db); err != nil {
		t.Fatalf("migrateConversationHistory: %v", err)
	}

	var msg entity.ConversationMessage
	if err := db.Table(conversationMessageTable).Where("conversation_id = ? AND position = ?", "c1", 0).Take(&msg).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if derefString(msg.Content) != "written by the runtime" {
		t.Fatalf("expected the runtime row to win, got %q", derefString(msg.Content))
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestMigrateConversationHistorySkipsTablesWithoutPayloadColumns(t *testing.T) {
	db := setupConversationHistoryMigrationDB(t, false)
	insertConversation(t, db, "conversation", "c1")

	if err := migrateConversationHistory(t.Context(), db); err != nil {
		t.Fatalf("migrateConversationHistory: %v", err)
	}

	if got := countHistoryRows(t, db, conversationMessageTable); got != 0 {
		t.Fatalf("expected no backfill without payload columns, got %d", got)
	}
	// The step still advances the marker: a schema without the payload columns
	// has nothing left to split, ever.
	if got := databaseVersion(t, db); got != conversationHistoryTargetVersion {
		t.Fatalf("expected database version %s, got %s", conversationHistoryTargetVersion, got)
	}
}

func TestMigrateConversationHistoryHonoursRecordedVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		migrate bool
	}{
		{"no recorded version", "", true},
		{"release line below the split", "v0.27.2", true},
		{"earlier dev build", "v1.0.0-rc1.dev0", true},
		{"the split version itself", conversationHistoryTargetVersion, false},
		{"release the dev build leads to", "v1.0.0-rc1", false},
		{"later dev build", "v1.0.0-rc1.dev2", false},
		{"later release candidate", "v1.0.0-rc2", false},
		{"final release", "v1.0.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupConversationHistoryMigrationDB(t, true)
			insertLegacyConversation(t, db, "conversation", "c1", `[{"role":"user","content":"hi"}]`, `[]`)
			if tc.current != "" {
				writeDatabaseVersion(t, db, tc.current)
			}

			if err := migrateConversationHistory(t.Context(), db); err != nil {
				t.Fatalf("migrateConversationHistory: %v", err)
			}

			var wantMessages int64
			wantVersion := tc.current
			if tc.migrate {
				wantMessages = 1
				wantVersion = conversationHistoryTargetVersion
			}
			if got := countHistoryRows(t, db, conversationMessageTable); got != wantMessages {
				t.Fatalf("expected %d messages, got %d", wantMessages, got)
			}
			if got := databaseVersion(t, db); got != wantVersion {
				t.Fatalf("expected database version %s, got %s", wantVersion, got)
			}
		})
	}
}

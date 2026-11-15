package dao

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestMigrateConversationReferences(t *testing.T) {
	for _, tables := range [][3]string{
		{"conversation", conversationMessageTable, conversationReferenceTable},
		{"api_4_conversation", apiConversationMessageTable, apiConversationReferenceTable},
	} {
		t.Run(tables[0], func(t *testing.T) {
			db := setupConversationHistoryMigrationDB(t, false)
			insertConversation(t, db, tables[0], "session")
			messages := []map[string]interface{}{
				{"role": "system", "content": "system"},
				{"role": "assistant", "content": "prologue"},
				{"role": "user", "id": "unanswered"},
			}
			// Cross a reference batch boundary within the same conversation.
			for i := 0; i < conversationHistoryBatchSize+3; i++ {
				messages = append(messages, map[string]interface{}{"role": "user"}, map[string]interface{}{"role": "assistant"})
			}
			messages = append(messages,
				map[string]interface{}{"role": "user", "id": "delayed"},
				map[string]interface{}{"role": "assistant", "id": "delayed"},
				map[string]interface{}{"role": "user", "id": "later"},
				map[string]interface{}{"role": "assistant", "id": "later", "reference": map[string]interface{}{"chunks": []interface{}{}, "saved": true}},
			)
			raw, err := json.Marshal(messages)
			if err != nil {
				t.Fatal(err)
			}
			if err := createHistory(t.Context(), db, tables[1], "message", "session", raw); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < conversationHistoryBatchSize+3; i++ {
				if err := db.Table(tables[2]).Create(map[string]interface{}{
					"conversation_id": "session", "position": i, "reference": fmt.Sprintf(`{"source":%d}`, i),
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			// Completion order differs from message order; message_position wins.
			for i, position := range []int{len(messages) - 1, len(messages) - 3} {
				if err := db.Table(tables[2]).Create(map[string]interface{}{
					"conversation_id": "session", "position": 500 + i, "message_position": position, "reference": `{"explicit":true}`,
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateMessageReferences(t.Context(), db, tables[1], tables[2]); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				// Retry copied rows, then retry after table deletion before the marker.
				writeDatabaseVersion(t, db, dslVersionTargetVersion)
				if err := migrateConversationReferences(t.Context(), db); err != nil {
					t.Fatal(err)
				}
				if got := databaseVersion(t, db); got != conversationReferenceTargetVersion {
					t.Fatalf("version = %s", got)
				}
				for _, table := range []string{conversationReferenceTable, apiConversationReferenceTable} {
					if db.Migrator().HasTable(table) {
						t.Fatalf("source table still exists: %s", table)
					}
				}
				var rows []entity.ConversationMessage
				if err := db.Table(tables[1]).Order("position").Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				for i, row := range rows {
					want := ""
					if i >= 4 && i < len(messages)-4 && i%2 == 0 {
						want = fmt.Sprintf(`{"source":%d}`, (i-4)/2)
					} else if i == len(messages)-3 {
						want = `{"explicit":true}`
					} else if i == len(messages)-1 {
						want = `{"chunks":[],"saved":true}`
					}
					if string(row.Reference) != want {
						t.Fatalf("message %d reference = %s, want %s", i, row.Reference, want)
					}
				}
			}
			writeDatabaseVersion(t, db, "v1.0.0-rc3.dev8")
			if err := migrateConversationReferences(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			if got := databaseVersion(t, db); got != "v1.0.0-rc3.dev8" {
				t.Fatalf("downgraded marker: %s", got)
			}
		})
	}
}

func TestConversationReferenceCleanupForMigratedDatabase(t *testing.T) {
	for _, version := range []string{conversationReferenceTargetVersion, "v1.0.0-rc3.dev8"} {
		t.Run(version, func(t *testing.T) {
			db := setupConversationHistoryMigrationDB(t, false)
			insertConversation(t, db, "conversation", "session")
			if err := createHistory(t.Context(), db, conversationMessageTable, "message", "session", json.RawMessage(`[{"role":"user"},{"role":"assistant"}]`)); err != nil {
				t.Fatal(err)
			}
			if err := db.Table(conversationReferenceTable).Create(map[string]interface{}{
				"conversation_id": "session", "position": 0, "reference": `{"copied":true}`,
			}).Error; err != nil {
				t.Fatal(err)
			}
			writeDatabaseVersion(t, db, version)
			if err := migrateConversationReferences(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			if got := databaseVersion(t, db); got != version {
				t.Fatalf("changed migration version: %s", got)
			}
			var answer entity.ConversationMessage
			if err := db.Where("conversation_id = ? AND position = ?", "session", 1).Take(&answer).Error; err != nil {
				t.Fatal(err)
			}
			if string(answer.Reference) != `{"copied":true}` {
				t.Fatalf("version marker prevented copying: %s", answer.Reference)
			}
			if err := autoMigrateRuntimeModels(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{conversationReferenceTable, apiConversationReferenceTable} {
				if db.Migrator().HasTable(table) {
					t.Fatalf("server recreated source table: %s", table)
				}
			}
		})
	}
}

func TestConversationReferenceCleanupSkipsUnmatchedReferences(t *testing.T) {
	db := setupConversationHistoryMigrationDB(t, false)
	insertConversation(t, db, "api_4_conversation", "unmatched")
	insertConversation(t, db, "conversation", "surviving")
	if err := createHistory(t.Context(), db, conversationMessageTable, "message", "surviving", json.RawMessage(`[{"role":"user"},{"role":"assistant"}]`)); err != nil {
		t.Fatal(err)
	}
	missingPosition := 9
	for _, row := range []struct {
		table, conversationID string
		position              int
		messagePosition       *int
		reference             string
	}{
		{apiConversationReferenceTable, "unmatched", 0, nil, `{"chunks":[{"id":"empty-history"}]}`},
		{conversationReferenceTable, "surviving", 0, nil, `{"chunks":[{"id":"surviving"}]}`},
		{conversationReferenceTable, "surviving", 1, nil, `{"chunks":[{"id":"surplus"}]}`},
		{conversationReferenceTable, "surviving", 9, &missingPosition, `{"chunks":[{"id":"missing-position"}]}`},
	} {
		if err := db.Table(row.table).Create(map[string]interface{}{
			"conversation_id": row.conversationID, "position": row.position, "message_position": row.messagePosition, "reference": row.reference,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	writeDatabaseVersion(t, db, dslVersionTargetVersion)
	if err := migrateConversationReferences(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{conversationReferenceTable, apiConversationReferenceTable} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("stale references prevented source cleanup: %s", table)
		}
	}
	if got := databaseVersion(t, db); got != conversationReferenceTargetVersion {
		t.Fatalf("migration version = %s", got)
	}
	var messages []entity.ConversationMessage
	if err := db.Order("position").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || string(messages[1].Reference) != `{"chunks":[{"id":"surviving"}]}` {
		t.Fatalf("stale references changed the surviving history: %+v", messages)
	}
	if err := migrateConversationReferences(t.Context(), db); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateConversationReferencesFailurePreservesVersion(t *testing.T) {
	db := setupConversationHistoryMigrationDB(t, false)
	insertConversation(t, db, "conversation", "session")
	if err := db.Table(conversationReferenceTable).Create(map[string]interface{}{
		"conversation_id": "session", "position": 0, "reference": "invalid JSON",
	}).Error; err != nil {
		t.Fatal(err)
	}
	writeDatabaseVersion(t, db, dslVersionTargetVersion)
	if err := migrateConversationReferences(t.Context(), db); err == nil {
		t.Fatal("invalid reference accepted")
	}
	if got := databaseVersion(t, db); got != dslVersionTargetVersion {
		t.Fatalf("advanced version after failure: %s", got)
	}
}

func TestMigrateConversationReferenceMetadata(t *testing.T) {
	for _, tables := range [][3]string{
		{"conversation", conversationMessageTable, conversationReferenceTable},
		{"api_4_conversation", apiConversationMessageTable, apiConversationReferenceTable},
	} {
		t.Run(tables[0], func(t *testing.T) {
			db := setupConversationHistoryMigrationDB(t, false)
			insertConversation(t, db, tables[0], "session")
			if err := createHistory(t.Context(), db, tables[1], "message", "session", json.RawMessage(`[{"role":"user"},{"role":"assistant","reference":[{"old":"inline chunks"}]},{"role":"assistant"}]`)); err != nil {
				t.Fatal(err)
			}
			// Existing flattened rows stored references in their metadata column.
			if err := db.Table(tables[1]).Where("conversation_id = ?", "session").Update("metadata", `{"reference":{"chunks":[]},"attachment":"file"}`).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Table(tables[2]).Create(map[string]interface{}{
				"conversation_id": "session", "position": 0, "reference": `{"chunks":[{"id":"source"}]}`,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := migrateConversationReferences(t.Context(), db); err != nil {
				t.Fatal(err)
			}
			var messages []entity.ConversationMessage
			if err := db.Table(tables[1]).Order("position").Find(&messages).Error; err != nil {
				t.Fatal(err)
			}
			for i, message := range messages {
				if message.Metadata != `{"attachment":"file"}` {
					t.Fatalf("metadata still contains reference: %s", message.Metadata)
				}
				want := `{"chunks":[]}`
				if i == 1 {
					want = `{"chunks":[{"id":"source"}]}`
				}
				if string(message.Reference) != want {
					t.Fatalf("message %d reference = %s, want %s", i, message.Reference, want)
				}
			}
		})
	}
}

func TestMessageReferencePersistenceAndProjection(t *testing.T) {
	for _, table := range []string{conversationMessageTable, apiConversationMessageTable} {
		t.Run(table, func(t *testing.T) {
			db := setupConversationHistoryMigrationDB(t, false)
			parent := "conversation"
			if table == apiConversationMessageTable {
				parent = "api_4_conversation"
			}
			insertConversation(t, db, parent, "session")
			for _, update := range []ConversationHistoryUpdate{
				{Message: map[string]interface{}{"id": "turn", "role": "user", "content": "question"}},
				{QuestionID: "turn", Message: map[string]interface{}{"id": "turn", "role": "assistant", "content": "answer", "prompt": "system prompt", "usage": map[string]interface{}{"total_tokens": 10}, "elapsed_ms": 50, "reference": map[string]interface{}{"chunks": []interface{}{map[string]interface{}{"id": "chunk"}}}}},
			} {
				if err := updateConversationHistory(t.Context(), db, table, "session", update); err != nil {
					t.Fatal(err)
				}
			}
			for _, options := range []ConversationHistoryOptions{
				{},
				{IncludeUsage: true, IncludeElapsedMs: true},
				{IncludeReference: true},
				{IncludePrompt: true},
				{IncludeReference: true, IncludePrompt: true, IncludeUsage: true, IncludeElapsedMs: true},
			} {
				var sql string
				capture := db.Session(&gorm.Session{DryRun: true})
				if err := capture.Callback().Query().After("gorm:query").Register("capture_history_sql", func(tx *gorm.DB) {
					sql = tx.Statement.SQL.String()
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := loadHistory(t.Context(), capture, table, []string{"session"}, options); err != nil {
					t.Fatal(err)
				}
				if err := capture.Callback().Query().Remove("capture_history_sql"); err != nil {
					t.Fatal(err)
				}
				for column, include := range map[string]bool{"reference": options.IncludeReference, "prompt": options.IncludePrompt, "usage": options.IncludeUsage, "elapsed_ms": options.IncludeElapsedMs} {
					if strings.Contains(sql, column) != include {
						t.Fatalf("options=%+v projection %s: %s", options, column, sql)
					}
				}
				if strings.Contains(sql, "SELECT *") || !strings.Contains(sql, "content") {
					t.Fatalf("invalid projection: %s", sql)
				}

				history, err := loadHistory(t.Context(), db, table, []string{"session"}, options)
				if err != nil {
					t.Fatal(err)
				}
				var messages []map[string]interface{}
				if err := json.Unmarshal(history["session"], &messages); err != nil {
					t.Fatal(err)
				}
				for key, include := range map[string]bool{"reference": options.IncludeReference, "prompt": options.IncludePrompt, "usage": options.IncludeUsage, "elapsed_ms": options.IncludeElapsedMs} {
					if _, ok := messages[1][key]; ok != include {
						t.Fatalf("options=%+v key %s in %s", options, key, history["session"])
					}
				}
			}
		})
	}
}

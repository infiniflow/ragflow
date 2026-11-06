package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
)

const conversationReferenceTargetVersion = "v1.0.0-rc3.dev7"

// migrateConversationReferences runs after the history split and all earlier
// versioned migrations. Source tables are dropped only after both copies succeed.
func migrateConversationReferences(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return err
	}
	if shouldSkipMigration(version, conversationReferenceTargetVersion) &&
		!db.WithContext(ctx).Migrator().HasTable(conversationReferenceTable) &&
		!db.WithContext(ctx).Migrator().HasTable(apiConversationReferenceTable) {
		return nil
	}
	for _, tables := range [][2]string{
		{conversationMessageTable, conversationReferenceTable},
		{apiConversationMessageTable, apiConversationReferenceTable},
	} {
		if err := migrateMessageReferences(ctx, db, tables[0], tables[1]); err != nil {
			return fmt.Errorf("migrate %s references: %w", tables[0], err)
		}
		if err := migrateMessageReferenceMetadata(ctx, db, tables[0]); err != nil {
			return fmt.Errorf("migrate %s reference metadata: %w", tables[0], err)
		}
	}
	for _, table := range []string{conversationReferenceTable, apiConversationReferenceTable} {
		if db.WithContext(ctx).Migrator().HasTable(table) {
			if err := db.WithContext(ctx).Migrator().DropTable(table); err != nil {
				return fmt.Errorf("drop migrated table %s: %w", table, err)
			}
		}
	}
	if shouldSkipMigration(version, conversationReferenceTargetVersion) {
		return nil
	}
	return setDatabaseMigrationVersion(ctx, db, conversationReferenceTargetVersion)
}

func migrateMessageReferences(ctx context.Context, db *gorm.DB, messageTable, referenceTable string) error {
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(referenceTable) {
		return nil
	}
	type referenceRow struct {
		ConversationID string
		Position       int
		MessagePos     *int `gorm:"column:message_position"`
		Reference      string
	}
	lastConversation, activeConversation := "", ""
	lastPosition, lastAssistant := -1, -1
	unmatched := 0
	for {
		var rows []referenceRow
		if err := db.Table(referenceTable).
			Select("conversation_id", "position", "message_position", "reference").
			Where("conversation_id > ? OR (conversation_id = ? AND position > ?)", lastConversation, lastConversation, lastPosition).
			Order("conversation_id, position").Limit(conversationHistoryBatchSize).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if row.ConversationID != activeConversation {
					activeConversation, lastAssistant = row.ConversationID, -1
				}
				if !json.Valid([]byte(row.Reference)) {
					return fmt.Errorf("invalid reference JSON for conversation %s position %d", row.ConversationID, row.Position)
				}
				position := row.MessagePos
				if position == nil {
					// Array references follow assistant order after the first user.
					// Explicit associations belong to later targeted writes and must
					// not consume an entry from the old reference array.
					var answer struct{ Position int }
					err := tx.Table(messageTable).Select("position").
						Where("conversation_id = ? AND role = ? AND position > ?", row.ConversationID, "assistant", lastAssistant).
						Where("position >= (SELECT MIN(position) FROM "+messageTable+" WHERE conversation_id = ? AND role = ?)", row.ConversationID, "user").
						Where("NOT EXISTS (SELECT 1 FROM "+referenceTable+" r WHERE r.conversation_id = ? AND r.message_position = "+messageTable+".position)", row.ConversationID).
						Order("position").Take(&answer).Error
					if errors.Is(err, gorm.ErrRecordNotFound) {
						unmatched++
						continue
					}
					if err != nil {
						return err
					}
					lastAssistant = answer.Position
					position = &answer.Position
				}
				query := tx.Table(messageTable).Where("conversation_id = ? AND position = ? AND role = ?", row.ConversationID, *position, "assistant")
				result := query.Session(&gorm.Session{}).Where("reference IS NULL OR reference = ? OR reference = ? OR TRIM(reference) LIKE ?", "", "null", "[%").Update("reference", row.Reference)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					var count int64
					if err := query.Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						unmatched++
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
		last := rows[len(rows)-1]
		lastConversation, lastPosition = last.ConversationID, last.Position
	}
	if unmatched > 0 {
		// References can outlive deleted messages or unsuccessful turns. They
		// cannot be attached to another answer without changing its citations.
		common.Warn("Skipped references without assistant messages during migration", zap.String("table", referenceTable), zap.Int("references", unmatched))
	}
	return nil
}

// Older message rows can carry a second copy in metadata. Move that value when
// there is no reference column value, then remove it so lean SQL reads stay lean.
func migrateMessageReferenceMetadata(ctx context.Context, db *gorm.DB, table string) error {
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(table) {
		return nil
	}
	lastConversation, lastPosition := "", -1
	for {
		var rows []struct {
			ConversationID string
			Position       int
			Metadata       string
		}
		if err := db.Table(table).Select("conversation_id", "position", "metadata").
			Where("metadata LIKE ?", `%"reference"%`).
			Where("conversation_id > ? OR (conversation_id = ? AND position > ?)", lastConversation, lastConversation, lastPosition).
			Order("conversation_id, position").Limit(conversationHistoryBatchSize).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				var metadata map[string]json.RawMessage
				if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
					return fmt.Errorf("decode metadata for conversation %s position %d: %w", row.ConversationID, row.Position, err)
				}
				reference, ok := metadata["reference"]
				if !ok {
					continue
				}
				delete(metadata, "reference")
				raw, err := json.Marshal(metadata)
				if err != nil {
					return err
				}
				query := tx.Table(table).Where("conversation_id = ? AND position = ?", row.ConversationID, row.Position)
				if err := query.Session(&gorm.Session{}).Where("reference IS NULL OR reference = ? OR reference = ?", "", "null").Update("reference", string(reference)).Error; err != nil {
					return err
				}
				if err := query.Update("metadata", string(raw)).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		last := rows[len(rows)-1]
		lastConversation, lastPosition = last.ConversationID, last.Position
	}
}

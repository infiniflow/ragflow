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

package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/common"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const connectorCredentialsMigrationBatchSize = 256

// The raw column text is read so the update can use it as an optimistic guard.
type connectorCredentialsMigrationRow struct {
	ID     string `gorm:"column:id"`
	Config string `gorm:"column:config"`
}

// migrateConnectorCredentials encrypts the plaintext credentials of existing
// connectors when RAGFLOW_CONNECTOR_KEY is set. A second run changes nothing.
func migrateConnectorCredentials(ctx context.Context, db *gorm.DB) error {
	key, err := common.ConnectorKey()
	if err != nil {
		return err
	}
	if key == nil || !db.WithContext(ctx).Migrator().HasTable("connector") {
		return nil
	}

	var migrated int
	var rows []connectorCredentialsMigrationRow
	result := db.WithContext(ctx).Table("connector").Select("id", "config").
		FindInBatches(&rows, connectorCredentialsMigrationBatchSize, func(_ *gorm.DB, _ int) error {
			for _, row := range rows {
				// The whole row is rewritten, so numbers must survive exactly; float64 rounds above 2^53.
				// Decode stops after the first value, so json.Valid keeps rows with trailing data skipped.
				if !json.Valid([]byte(row.Config)) {
					continue
				}
				var config map[string]interface{}
				dec := json.NewDecoder(strings.NewReader(row.Config))
				dec.UseNumber()
				if err := dec.Decode(&config); err != nil {
					continue
				}
				if _, ok := config["credentials"]; !ok || common.HasEncryptedConnectorCredentials(config) {
					continue
				}
				encrypted, err := common.EncryptConnectorCredentials(config)
				if err != nil {
					return fmt.Errorf("encrypt connector %q credentials: %w", row.ID, err)
				}
				value, err := json.Marshal(encrypted)
				if err != nil {
					return fmt.Errorf("encode connector %q config: %w", row.ID, err)
				}
				updated, err := updateConnectorConfigIfUnchanged(ctx, db, row.ID, row.Config, string(value))
				if err != nil {
					return fmt.Errorf("update connector %q config: %w", row.ID, err)
				}
				if updated {
					migrated++
				}
			}
			return nil
		})
	if result.Error != nil {
		return result.Error
	}
	if migrated > 0 {
		common.Info("Encrypted connector credentials", zap.Int("connectors", migrated))
	}
	return nil
}

// updateConnectorConfigIfUnchanged writes newConfig only while the row still
// holds oldConfig, so it never overwrites a change made after the read.
func updateConnectorConfigIfUnchanged(ctx context.Context, db *gorm.DB, id, oldConfig, newConfig string) (bool, error) {
	result := db.WithContext(ctx).Table("connector").
		Where("id = ? AND config = ?", id, oldConfig).
		Update("config", newConfig)
	return result.RowsAffected == 1, result.Error
}

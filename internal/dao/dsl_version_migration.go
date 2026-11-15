package dao

import (
	"context"
	"fmt"

	"ragflow/internal/common"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// dslVersionTargetVersion marks the backfill of pipeline_version_id bindings as
// done. It sorts after every existing migration target version, so the step runs
// exactly once and is skipped on later starts.
const dslVersionTargetVersion = "v1.0.0-rc3.dev6"

// migrateDSLVersionColumns adds the pipeline_version_id column to the tables that
// bind a dataset (knowledgebase) or document to a dataflow canvas version, and
// ensures user_canvas_version.alias exists (design A2/F1). It is
// idempotent: an already-present column is left untouched, and a concurrent add
// that loses the race to a duplicate-column error is tolerated.
func MigrateDSLVersionColumns(ctx context.Context, db *gorm.DB) error {
	for _, table := range []string{"knowledgebase", "document"} {
		if !db.WithContext(ctx).Migrator().HasTable(table) {
			continue
		}
		exists, err := columnExists(ctx, db, table, "pipeline_version_id")
		if err != nil {
			return fmt.Errorf("check %s.pipeline_version_id: %w", table, err)
		}
		if exists {
			continue
		}
		if err := db.WithContext(ctx).Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN pipeline_version_id varchar(32) NULL", table)).Error; err != nil {
			if isDuplicateColumnError(err) {
				continue
			}
			return fmt.Errorf("add %s.pipeline_version_id: %w", table, err)
		}
		common.Info("Added " + table + ".pipeline_version_id")
	}

	// The dataflow version table carries a free-text alias (design A2/F1). The
	// column is defined on the entity and is normally created by AutoMigrate, but
	// a database whose user_canvas_version table predates the column is missing
	// it, which makes every version save fail with Error 1054. Add it explicitly
	// and idempotently, mirroring the pipeline_version_id handling above.
	if db.WithContext(ctx).Migrator().HasTable("user_canvas_version") {
		exists, err := columnExists(ctx, db, "user_canvas_version", "alias")
		if err != nil {
			return fmt.Errorf("check user_canvas_version.alias: %w", err)
		}
		if !exists {
			if err := db.WithContext(ctx).Exec(
				"ALTER TABLE user_canvas_version ADD COLUMN alias varchar(255) NULL").Error; err != nil {
				if !isDuplicateColumnError(err) {
					return fmt.Errorf("add user_canvas_version.alias: %w", err)
				}
			} else {
				common.Info("Added user_canvas_version.alias")
			}
		}
	}
	return nil
}

// backfillDSLVersionBindings locks every pre-existing dataflow-canvas binding to
// the latest version of that canvas at migration time. It is gated by the shared
// database migration-version marker so it runs once. Bindings to a built-in
// parser id (no backing user_canvas), to a non-dataflow canvas, or to a canvas
// that has no versions yet are left unbound; a dangling version id is never
// written.
func BackfillDSLVersionBindings(ctx context.Context, db *gorm.DB) error {
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read db migration version: %w", err)
	}
	if shouldSkipMigration(currentVersion, dslVersionTargetVersion) {
		common.Info("DSL version bindings already backfilled, skipping",
			zap.String("current_version", currentVersion),
			zap.String("target_version", dslVersionTargetVersion))
		return nil
	}

	tables := []string{"knowledgebase", "document"}
	hasAny := false
	for _, table := range tables {
		if db.WithContext(ctx).Migrator().HasTable(table) {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return setDatabaseMigrationVersion(ctx, db, dslVersionTargetVersion)
	}

	for _, table := range tables {
		if !db.WithContext(ctx).Migrator().HasTable(table) {
			continue
		}
		// Cache per-canvas resolution so a large table with few distinct canvases
		// does not issue a GetByID + GetLatest round trip per row (N+1 during
		// RunMigrations). The cached value is one of:
		//   skip=true     -> not a dataflow canvas (or no versions): skip the binding
		//   skip=false    -> versionID is the version to pin
		type canvasResolution struct {
			skip      bool
			versionID string
		}
		resolved := map[string]canvasResolution{}
		resolveCanvas := func(canvasID string) canvasResolution {
			if r, ok := resolved[canvasID]; ok {
				return r
			}
			r := canvasResolution{skip: true}
			canvas, cerr := NewUserCanvasDAO().GetByID(ctx, db, canvasID)
			if cerr != nil || canvas == nil {
				// Not a user canvas (e.g. a built-in parser id) -> no version binding.
				resolved[canvasID] = r
				return r
			}
			if canvas.CanvasCategory != "dataflow_canvas" {
				// Only dataflow canvases carry versions; agent canvases are excluded.
				resolved[canvasID] = r
				return r
			}
			version, verr := NewUserCanvasVersionDAO().GetLatest(ctx, db, canvasID)
			if verr != nil {
				// Canvas has no versions yet -> leave unbound, never write a dangling id.
				common.Warn("backfill: canvas has no versions, skipping binding",
					zap.String("canvas_id", canvasID),
					zap.String("table", table))
				resolved[canvasID] = r
				return r
			}
			r = canvasResolution{skip: false, versionID: version.ID}
			resolved[canvasID] = r
			return r
		}

		var rows []struct {
			ID         string  `gorm:"column:id"`
			PipelineID *string `gorm:"column:pipeline_id"`
		}
		result := db.WithContext(ctx).Table(table).
			Where("pipeline_id IS NOT NULL AND pipeline_id <> ''").
			Select("id, pipeline_id").
			FindInBatches(&rows, 256, func(batchDB *gorm.DB, _ int) error {
				return batchDB.Transaction(func(tx *gorm.DB) error {
					for _, row := range rows {
						if row.PipelineID == nil {
							continue
						}
						res := resolveCanvas(*row.PipelineID)
						if res.skip {
							continue
						}
						if err := tx.Table(table).Where("id = ?", row.ID).Update("pipeline_version_id", res.versionID).Error; err != nil {
							return fmt.Errorf("backfill update %s %s: %w", table, row.ID, err)
						}
					}
					return nil
				})
			})
		if result.Error != nil {
			return fmt.Errorf("backfill %s: %w", table, result.Error)
		}
	}

	if err := setDatabaseMigrationVersion(ctx, db, dslVersionTargetVersion); err != nil {
		return fmt.Errorf("set db migration version %s: %w", dslVersionTargetVersion, err)
	}
	return nil
}

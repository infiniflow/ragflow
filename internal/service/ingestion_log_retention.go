package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ingestionLogFoldResult struct {
	Before        int
	Protected     int
	DeletedRows   int
	OmittedEvents int
	After         int
	SummaryID     int
	Skipped       bool
	SkipReason    string
}

type ingestionDocumentTrimResult struct {
	BeforeEvents  int
	DeletedRuns   int
	DeletedEvents int
	AfterEvents   int
}

func logIngestionFoldResult(taskID, pipelineLogID string, result ingestionLogFoldResult, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("event", "ingestion_log_fold"),
		zap.String("task_id", taskID),
		zap.String("pipeline_log_id", pipelineLogID),
		zap.Int("before", result.Before),
		zap.Int("after", result.After),
		zap.Int("deleted", result.DeletedRows),
		zap.Int("omitted", result.OmittedEvents),
		zap.Int("protected", result.Protected),
		zap.Int("summary_id", result.SummaryID),
		zap.Duration("duration", duration),
	}
	if err != nil {
		fields = append(fields, zap.String("result", "failure"))
		common.Error("ingestion log fold failed", err, fields...)
		return
	}
	if result.Skipped {
		fields = append(fields, zap.String("result", "skipped"), zap.String("reason", result.SkipReason))
		common.Info("ingestion log fold skipped", fields...)
		return
	}
	fields = append(fields, zap.String("result", "success"))
	common.Info("ingestion log fold completed", fields...)
}

func logIngestionTrimResult(documentID, taskID, preservePipelineLogID string, result ingestionDocumentTrimResult, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("event", "ingestion_log_run_delete"),
		zap.String("document_id", documentID),
		zap.String("task_id", taskID),
		zap.String("preserve_pipeline_log_id", preservePipelineLogID),
		zap.Int("before", result.BeforeEvents),
		zap.Int("after", result.AfterEvents),
		zap.Int("deleted_runs", result.DeletedRuns),
		zap.Int("deleted_events", result.DeletedEvents),
		zap.Duration("duration", duration),
	}
	if err != nil {
		fields = append(fields, zap.String("result", "failure"))
		common.Error("ingestion log run deletion failed", err, fields...)
		return
	}
	fields = append(fields, zap.String("result", "success"), zap.String("reason", "document_event_cap"))
	common.Info("ingestion log run deletion completed", fields...)
}

// foldIngestionRun compacts one terminal run without changing the run's
// lifecycle truth. The function is intentionally private: callers must first
// settle the run and then invoke this same implementation, rather than
// maintaining a second cleanup path.
func (s *IngestionTaskService) foldIngestionRun(ctx context.Context, pipelineLogID string, maxRows int) (ingestionLogFoldResult, error) {
	if pipelineLogID == "" {
		return ingestionLogFoldResult{}, errors.New("fold ingestion run requires a pipeline log id")
	}
	if maxRows < 2 {
		return ingestionLogFoldResult{}, errors.New("fold ingestion run requires max rows >= 2")
	}

	var result ingestionLogFoldResult
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run entity.PipelineOperationLog
		if err := tx.WithContext(ctx).Where("id = ?", pipelineLogID).First(&run).Error; err != nil {
			return err
		}

		var events []*entity.IngestionTaskLog
		if err := tx.WithContext(ctx).
			Where("pipeline_log_id = ?", pipelineLogID).
			Order("id ASC").Find(&events).Error; err != nil {
			return err
		}
		result.Before = len(events)
		result.After = result.Before

		var latestTerminal *entity.IngestionTaskLog
		var latestSummary *entity.IngestionTaskLog
		latestLifecycle := make(map[string]*entity.IngestionTaskLog)
		for _, event := range events {
			switch event.EventType {
			case dao.EventTypeLifecycle:
				if event.Component != "" {
					latestLifecycle[event.Component] = event
				}
			case dao.EventTypeTerminal:
				latestTerminal = event
			case dao.EventTypeSystem:
				if latestSummary == nil || event.ID > latestSummary.ID {
					latestSummary = event
				}
			}
		}

		if latestSummary != nil {
			result.SummaryID = latestSummary.ID
		}
		if !isTerminalPipelineOperationStatus(run.OperationStatus) {
			result.Skipped = true
			result.SkipReason = "run_not_terminal"
			return nil
		}
		if result.Before <= maxRows {
			return nil
		}

		protected := make(map[int]struct{}, len(latestLifecycle)+2)
		for _, event := range latestLifecycle {
			protected[event.ID] = struct{}{}
		}
		if latestTerminal != nil {
			protected[latestTerminal.ID] = struct{}{}
		}
		if latestSummary != nil {
			protected[latestSummary.ID] = struct{}{}
		}
		result.Protected = len(protected)
		if result.Protected >= maxRows {
			result.Skipped = true
			result.SkipReason = "protected_rows_reach_limit"
			return nil
		}

		ordinary := make([]*entity.IngestionTaskLog, 0, len(events)-len(protected))
		for _, event := range events {
			if _, keep := protected[event.ID]; !keep {
				ordinary = append(ordinary, event)
			}
		}

		summaryReserved := 0
		if latestSummary == nil {
			summaryReserved = 1
		}
		ordinaryBudget := maxRows - result.Protected - summaryReserved
		if ordinaryBudget < 0 {
			result.Skipped = true
			result.SkipReason = "protected_rows_reach_limit"
			return nil
		}

		// Keep the newest ordinary messages first, then use the remaining
		// budget for the oldest ordinary messages. Dynamic 4/5 tail and 1/5
		// head allocation scales to any configured cap while preserving the
		// 4,000 / 999 balance under the default 5,000-row cap.
		tailBudget := (ordinaryBudget*4 + 4) / 5
		if tailBudget > ordinaryBudget {
			tailBudget = ordinaryBudget
		}
		headBudget := ordinaryBudget - tailBudget
		keepOrdinary := make(map[int]struct{}, headBudget+tailBudget)
		for _, event := range ordinary[:headBudget] {
			keepOrdinary[event.ID] = struct{}{}
		}
		for _, event := range ordinary[len(ordinary)-tailBudget:] {
			keepOrdinary[event.ID] = struct{}{}
		}

		deleteEvents := make([]*entity.IngestionTaskLog, 0, len(ordinary))
		for _, event := range ordinary {
			if _, keep := keepOrdinary[event.ID]; !keep {
				deleteEvents = append(deleteEvents, event)
			}
		}
		if len(deleteEvents) == 0 {
			return nil
		}

		newSummary := latestSummary == nil
		if newSummary {
			latestSummary = deleteEvents[0]
			result.SummaryID = latestSummary.ID
			deleteEvents = deleteEvents[1:]
		}
		omittedEvents := len(deleteEvents) + boolToInt(newSummary)
		omittedRows := deleteEvents
		if newSummary {
			omittedRows = append([]*entity.IngestionTaskLog{latestSummary}, deleteEvents...)
		}
		summaryMessage := formatIngestionFoldMessage(omittedEvents, omittedRows, latestSummary)
		if err := tx.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
			Where("id = ?", latestSummary.ID).
			Updates(map[string]interface{}{
				"checkpoint": entity.JSONMap{},
				"event_type": dao.EventTypeSystem,
				"component":  "",
				"phase":      0,
				"message":    summaryMessage,
			}).Error; err != nil {
			return err
		}

		deleteIDs := make([]int, 0, len(deleteEvents))
		for _, event := range deleteEvents {
			deleteIDs = append(deleteIDs, event.ID)
		}
		if len(deleteIDs) > 0 {
			if err := tx.WithContext(ctx).Where("id IN ?", deleteIDs).Delete(&entity.IngestionTaskLog{}).Error; err != nil {
				return err
			}
		}
		result.DeletedRows = len(deleteIDs)
		result.OmittedEvents = omittedEvents

		var after int64
		if err := tx.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
			Where("pipeline_log_id = ?", pipelineLogID).Count(&after).Error; err != nil {
			return err
		}
		result.After = int(after)
		return nil
	})
	return result, err
}

// trimIngestionDocument enforces the cross-run event cap by deleting complete
// oldest terminal runs. It deliberately leaves pipeline_operation_log rows in
// place: run_count is the document's immutable history ledger. The latest run
// and the caller's run are never deleted, even if a late terminal writer is
// settling an older delivery.
func (s *IngestionTaskService) trimIngestionDocument(ctx context.Context, documentID, preservePipelineLogID string, maxRows int) (ingestionDocumentTrimResult, error) {
	if documentID == "" {
		return ingestionDocumentTrimResult{}, errors.New("trim ingestion document requires a document id")
	}
	if maxRows <= 0 {
		return ingestionDocumentTrimResult{}, errors.New("trim ingestion document requires positive max rows")
	}

	var result ingestionDocumentTrimResult
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runs []*entity.PipelineOperationLog
		if err := tx.WithContext(ctx).
			Where("document_id = ? AND run_count IS NOT NULL AND run_count > 0", documentID).
			Order("run_count ASC").Order("create_time ASC").Order("id ASC").
			Find(&runs).Error; err != nil {
			return err
		}
		if len(runs) == 0 {
			return nil
		}

		latestRunID := runs[len(runs)-1].ID
		preserved := map[string]struct{}{latestRunID: {}}
		if preservePipelineLogID != "" {
			preserved[preservePipelineLogID] = struct{}{}
		}

		eventCounts := make(map[string]int, len(runs))
		for _, run := range runs {
			var count int64
			if err := tx.WithContext(ctx).Model(&entity.IngestionTaskLog{}).
				Where("pipeline_log_id = ?", run.ID).Count(&count).Error; err != nil {
				return err
			}
			eventCounts[run.ID] = int(count)
			result.BeforeEvents += int(count)
		}
		result.AfterEvents = result.BeforeEvents

		for _, run := range runs {
			if result.AfterEvents <= maxRows {
				break
			}
			if _, keep := preserved[run.ID]; keep || !isTerminalPipelineOperationStatus(run.OperationStatus) {
				continue
			}
			count := eventCounts[run.ID]
			if count == 0 {
				continue
			}
			if err := tx.WithContext(ctx).Where("pipeline_log_id = ?", run.ID).Delete(&entity.IngestionTaskLog{}).Error; err != nil {
				return err
			}
			result.DeletedRuns++
			result.DeletedEvents += count
			result.AfterEvents -= count
		}
		return nil
	})
	return result, err
}

func isTerminalPipelineOperationStatus(status string) bool {
	switch status {
	case string(entity.TaskStatusDone), string(entity.TaskStatusFail), string(entity.TaskStatusCancel), "DONE", "FAIL", "CANCEL":
		return true
	default:
		return false
	}
}

func formatIngestionFoldMessage(omitted int, omittedRows []*entity.IngestionTaskLog, summary *entity.IngestionTaskLog) string {
	first := summary
	last := summary
	if len(omittedRows) > 0 {
		first = omittedRows[0]
		last = omittedRows[len(omittedRows)-1]
	}
	return fmt.Sprintf("… this run omitted %d process messages (time range %s–%s) …", omitted, foldEventTime(first), foldEventTime(last))
}

func foldEventTime(event *entity.IngestionTaskLog) string {
	if event == nil {
		return "unknown"
	}
	if event.CreateDate != nil {
		return event.CreateDate.Local().Format("15:04:05")
	}
	if event.CreateTime != nil {
		return time.UnixMilli(*event.CreateTime).Local().Format("15:04:05")
	}
	return "unknown"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

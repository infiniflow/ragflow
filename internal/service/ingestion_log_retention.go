package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const (
	defaultIngestionLogHeadRows = 999
	defaultIngestionLogTailRows = 4_000
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
		// budget for the oldest ordinary messages. With the default cap this
		// is 4,000 tail rows plus 999 head rows, less any protected rows.
		tailBudget := minInt(defaultIngestionLogTailRows, ordinaryBudget)
		headBudget := minInt(defaultIngestionLogHeadRows, ordinaryBudget-tailBudget)
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

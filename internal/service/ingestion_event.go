package service

import (
	"time"

	"ragflow/internal/entity"
)

// IngestionEventItem is the API representation of one persisted ingestion
// event. It is deliberately a one-to-one projection: callers must not join or
// rewrite messages into a synthetic status string.
type IngestionEventItem struct {
	ID        int        `json:"id"`
	TS        *time.Time `json:"ts"`
	EventType int        `json:"event_type"`
	Component string     `json:"component"`
	Phase     int        `json:"phase"`
	Message   string     `json:"message"`
}

func IngestionEventItemFromLog(event *entity.IngestionTaskLog) IngestionEventItem {
	if event == nil {
		return IngestionEventItem{}
	}
	return IngestionEventItem{
		ID:        event.ID,
		TS:        event.CreateDate,
		EventType: event.EventType,
		Component: event.Component,
		Phase:     event.Phase,
		Message:   event.Message,
	}
}

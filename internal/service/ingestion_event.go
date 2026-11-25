package service

import (
	"fmt"
	"time"

	"ragflow/internal/progressstore"
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
	ErrorCode string     `json:"error_code,omitempty"`
}

// unitLabels maps a structured unit tag to its UI label for the one-line
// progress message. Unknown tags are shown verbatim.
var unitLabels = map[string]string{
	"page:parse":      "页面解析",
	"page:refine":     "页面精炼",
	"chunk:final":     "文本切分",
	"chunk:keywords":  "关键词提取",
	"chunk:tags":      "标签提取",
	"chunk:resume":    "断点恢复",
	"chunk:remaining": "信息补全",
	"chunk:embed":     "向量化",
	"chunk:claim":     "观点抽取",
	"entity:generate": "实体生成",
	"batch:map":       "批量映射",
	"job:plan":        "规划任务",
	"row:extract":     "行抽取",
	"row:dedup":       "行去重",
}

// IngestionEventItemFromSnapshot derives a one-line progress message from a
// Kvrocks snapshot, e.g. "Extractor · 标签提取 3123/5047". It prefers the
// in-flight stage over a completed one that merely retains its last active unit.
func IngestionEventItemFromSnapshot(snap *progressstore.Snapshot) IngestionEventItem {
	if snap == nil {
		return IngestionEventItem{}
	}
	// Scan newest-first. A completed stage retains its Active unit, so a
	// forward scan would freeze on the first stage and hide the one actually
	// in flight — or the stage that failed. A running stage still wins over a
	// newer terminal one; otherwise the newest stage with a valid position
	// (including a failed one) is shown. This mirrors mergeProgressSnapshot's
	// selection in dataset/progress.go.
	var active, fallback *progressstore.Stage
	for i := len(snap.Stages) - 1; i >= 0; i-- {
		stage := &snap.Stages[i]
		if stage.Active == "" {
			continue
		}
		if _, ok := stage.Units[stage.Active]; !ok {
			continue
		}
		if fallback == nil {
			fallback = stage
		}
		if stage.Phase == progressstore.PhaseRunning {
			active = stage
			break
		}
	}
	if active == nil {
		active = fallback
	}
	if active == nil {
		return IngestionEventItem{}
	}
	pos := active.Units[active.Active]
	label := active.Active
	if friendly, ok := unitLabels[active.Active]; ok {
		label = friendly
	}
	return IngestionEventItem{
		Component: active.Name,
		Message:   active.Name + " · " + label + " " + fmt.Sprintf("%d/%d", pos.Current, pos.Total),
	}
}

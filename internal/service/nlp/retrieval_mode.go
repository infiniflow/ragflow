package nlp

import (
	"encoding/json"
	"fmt"
)

// RetrievalMode 指定检索候选的召回路径。
type RetrievalMode string

const (
	RetrievalModeHybrid  RetrievalMode = "hybrid"
	RetrievalModeVector  RetrievalMode = "vector"
	RetrievalModeKeyword RetrievalMode = "keyword"
)

func (m RetrievalMode) Validate() error {
	switch m {
	case RetrievalModeHybrid, RetrievalModeVector, RetrievalModeKeyword:
		return nil
	default:
		return fmt.Errorf("retrieval_mode 必须为 hybrid、vector 或 keyword，收到 %q", m)
	}
}

func (m *RetrievalMode) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("retrieval_mode 必须为字符串：%w", err)
	}
	mode := RetrievalMode(value)
	if err := mode.Validate(); err != nil {
		return err
	}
	*m = mode
	return nil
}

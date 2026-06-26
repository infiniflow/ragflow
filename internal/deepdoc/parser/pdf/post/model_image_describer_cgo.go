//go:build cgo

package post

import (
	modelModule "ragflow/internal/entity/models"
)

// modelDriverAdapter wraps a modelModule.ModelDriver as ChatDriver.
type modelDriverAdapter struct {
	d modelModule.ModelDriver
}

func (a *modelDriverAdapter) ChatWithMessages(
	modelName string, msgs []ChatMessage, apiCfg *ChatAPIConfig, chatCfg *ChatConfig,
) (*ChatResponse, error) {
	internal := make([]modelModule.Message, len(msgs))
	for i, m := range msgs {
		internal[i] = modelModule.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  m.ToolCalls,
		}
	}
	var mcCfg *modelModule.APIConfig
	if apiCfg != nil {
		mcCfg = &modelModule.APIConfig{
			ApiKey:  apiCfg.ApiKey,
			Region:  apiCfg.Region,
			BaseURL: apiCfg.BaseURL,
		}
	}
	resp, err := a.d.ChatWithMessages(modelName, internal, mcCfg, nil)
	if err != nil {
		return nil, err
	}
	var chatResp ChatResponse
	if resp.Answer != nil {
		chatResp.Answer = resp.Answer
	}
	if resp.ReasonContent != nil {
		chatResp.ReasonContent = resp.ReasonContent
	}
	chatResp.ToolCalls = resp.ToolCalls
	return &chatResp, nil
}

// newModelImageDescriberFromDriver creates a ModelImageDescriber from
// the real entity/models types. Used by post_steps_cgo.go.
func newModelImageDescriberFromDriver(
	d modelModule.ModelDriver, name string, cfg *modelModule.APIConfig, maxTokens int,
) *ModelImageDescriber {
	adapter := &modelDriverAdapter{d: d}
	var mirrorCfg *ChatAPIConfig
	if cfg != nil {
		mirrorCfg = &ChatAPIConfig{
			ApiKey:  cfg.ApiKey,
			Region:  cfg.Region,
			BaseURL: cfg.BaseURL,
		}
	}
	return NewModelImageDescriber(adapter, name, mirrorCfg, maxTokens)
}

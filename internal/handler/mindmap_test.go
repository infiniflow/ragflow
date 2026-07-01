package handler

import (
	modelModule "ragflow/internal/entity/models"
)

type fakeChatLLM struct {
	response     string
	err          error
	lastTenantID string
	lastModelID  string
	lastMessages []modelModule.Message
	lastConfig   *modelModule.ChatConfig
}

func (f *fakeChatLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	f.lastTenantID = tenantID
	f.lastModelID = modelID
	f.lastMessages = messages
	f.lastConfig = config
	if f.err != nil {
		return nil, f.err
	}
	answer := f.response
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

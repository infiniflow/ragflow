package service

import (
	"context"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/permission"
)

// GetAgenticRAGConfig resolves the chat's saved override against current defaults.
func (s *ChatService) GetAgenticRAGConfig(ctx context.Context, userID, chatID string) (*agentic_rag.Settings, error) {
	chat, err := s.getChatForOperation(ctx, userID, chatID, permission.OperationRead)
	if err != nil {
		return nil, err
	}
	override, err := agentic_rag.ParseOverride(chat.PromptConfig["agentic_rag"])
	if err != nil {
		return nil, err
	}
	return agentic_rag.GetSettings(ctx, override)
}

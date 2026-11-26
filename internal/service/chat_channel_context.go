package service

import (
	"context"
	"errors"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

// ChatChannelExecutionContext identifies a saved channel and its external conversation.
type ChatChannelExecutionContext struct {
	ChannelID string `json:"channel_id"`
	ChatID    string `json:"chat_id"`
}

func agentSessionHistoryUserID(ctx context.Context, targetID, sessionID string) string {
	if session, ok := ctx.Value(channelAgentSessionContextKey{}).(channelAgentSession); ok &&
		session.sessionID == sessionID && session.targetID == targetID {
		return session.userID
	}
	return permission.UserIDFromContext(ctx)
}

type channelChatSessionContextKey struct{}

type channelAgentSessionContextKey struct{}

type channelAgentSession struct {
	sessionID string
	targetID  string
	userID    string
}

// PrepareChatChannelCompletion authorizes channel execution under the caller's identity.
func PrepareChatChannelCompletion(ctx context.Context, execution ChatChannelExecutionContext, targetID, question string, agent bool) (context.Context, string, error) {
	if strings.TrimSpace(execution.ChannelID) == "" || strings.TrimSpace(execution.ChatID) == "" || targetID == "" {
		return ctx, "", common.NewCodedError(common.CodeArgumentError, "channel_context requires channel_id, chat_id, and a completion target")
	}
	checker := permission.NewDatabaseChecker(dao.DB)
	if err := checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: execution.ChannelID}, permission.OperationUse); err != nil {
		return ctx, "", err
	}
	channel, err := dao.NewChatChannel().GetByID(ctx, dao.DB, execution.ChannelID)
	if err != nil {
		return ctx, "", err
	}
	if channel.Status != 1 {
		return ctx, "", errors.New("chat channel is disabled")
	}
	target := channel.ChatID
	if agent {
		target = channel.AgentID
	}
	if target == nil || *target != targetID {
		return ctx, "", common.NewCodedError(common.CodeArgumentError, "completion target does not match the chat channel binding")
	}
	if err := validateChannelBinding(ctx, channel.ChatID, channel.AgentID); err != nil {
		return ctx, "", err
	}
	sessionID := channelSessionID(targetID, channel.ID, execution.ChatID)
	if agent {
		session, err := dao.NewAPI4ConversationDAO().GetMetadataBySessionID(ctx, dao.DB, sessionID, targetID)
		if err != nil {
			return ctx, "", err
		}
		ownerID := permission.UserIDFromContext(ctx)
		if session != nil {
			if session.Source == nil || *session.Source != string(entity.AgentSessionSourceChatChannel) {
				return ctx, "", permission.ErrPermissionDenied
			}
			ownerID = session.UserID
		}
		ctx = context.WithValue(ctx, channelAgentSessionContextKey{}, channelAgentSession{sessionID: sessionID, targetID: targetID, userID: ownerID})
		ctx = WithAgentSessionID(ctx, sessionID)
		ctx = context.WithValue(ctx, channelAgentRunContextKey{}, true)
		return withAgentChannelName(ctx, channel.Name), sessionID, nil
	}
	if err := checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChat, ID: targetID}, permission.OperationRun); err != nil {
		return ctx, "", err
	}
	session, err := NewChatSessionService().GetOrCreateForChannel(ctx, targetID, channel.ID, execution.ChatID, question, channel.Name)
	if err != nil {
		return ctx, "", err
	}
	if session.DialogID != targetID || session.Source != entity.ChatSessionSourceChatChannel {
		return ctx, "", permission.ErrPermissionDenied
	}
	return context.WithValue(ctx, channelChatSessionContextKey{}, session.ID), session.ID, nil
}

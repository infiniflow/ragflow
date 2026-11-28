//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build !ee

package service

import (
	"context"
	"fmt"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	serverconfig "ragflow/internal/server/config"

	"gorm.io/gorm"
)

func checkAgentSessionDeleteAccess(ctx context.Context, agentID string) (string, bool, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindCanvas,
		ID:   agentID,
	}, permission.OperationUse); err != nil {
		return "", false, err
	}
	return permission.UserIDFromContext(ctx), true, nil
}

func checkAgentDeleteAccess(ctx context.Context, canvasID string) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindCanvas,
		ID:   canvasID,
	}, permission.OperationDelete)
}

// CheckDocumentWrite checks whether the caller can write the document.
func CheckDocumentWrite(ctx context.Context, documentID, datasetID string, datasetOperation, documentOperation permission.Operation) error {
	checker := permission.NewDatabaseChecker(dao.DB)
	if datasetOperation != "" {
		if err := checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, datasetOperation); err != nil {
			return err
		}
	}
	if documentOperation == "" {
		return nil
	}
	return checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDocument, ID: documentID}, documentOperation)
}

// CheckDocumentManage preserves the community permission checks.
func CheckDocumentManage(ctx context.Context, documentID, datasetID string, datasetOperation, documentOperation permission.Operation) error {
	checker := permission.NewDatabaseChecker(dao.DB)
	if datasetOperation != "" {
		if err := checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, datasetOperation); err != nil {
			return err
		}
	}
	if documentOperation == "" {
		return nil
	}
	return checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDocument, ID: documentID}, documentOperation)
}

// configExtras is empty in the community edition.
type configExtras struct{}

func ingestionErrorCode(error) string { return "" }

// loadConfigExtras returns no additional settings in the community edition.
func loadConfigExtras(context.Context) (configExtras, error) { return configExtras{}, nil }

// consumeRegistrationProof is a no-op in the community edition.
func consumeRegistrationProof(context.Context, string) (common.ErrorCode, error) {
	return common.CodeSuccess, nil
}

// InitHTTPClientEE is a no-op in the community edition.
func (h *AdminClient) InitHTTPClientEE() error { return nil }

// pickGroupModel is unsupported in the community edition: model groups are an
// enterprise feature, so every gp- model reference fails resolution.
func (s *ModelFactory) pickGroupModel(_ context.Context, _, _ string, _ entity.ModelType) (*entity.TenantModel, error) {
	return nil, fmt.Errorf("%w: model group references are not supported", errModelConfigUnavailable)
}

// modelGroupDisplayName is unsupported in the community edition: model groups
// are an enterprise feature, so a gp- reference has no group name to resolve.
func modelGroupDisplayName(_ context.Context, _, _ string) (string, error) {
	return "", fmt.Errorf("%w: model group references are not supported", errModelConfigUnavailable)
}

func authorizeNonOwnerModelConfiguration(context.Context, *entity.TenantModelProvider, *entity.TenantModelInstance, []*entity.TenantModel) error {
	return errModelConfigUnavailable
}

func authorizeModelMetadataReadEE(_ context.Context, identity *modelIdentity) error {
	if identity == nil || identity.providerEntity == nil {
		return errModelConfigUnavailable
	}
	return nil
}

func (m *ModelProviderService) listModelProvidersForTenantEE(ctx context.Context, query *gorm.DB, tenantID string) ([]*entity.TenantModelProvider, error) {
	tenantOwner, err := m.userTenantDAO.GetOwnerByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, err
	}
	return m.modelProviderDAO.ListByTenantIDAndUserID(ctx, query, tenantID, tenantOwner.UserID)
}

func (*ModelProviderService) setModelOwnerInfoEE(context.Context, []map[string]interface{}, map[string]*entity.TenantModelProvider) error {
	return nil
}

func (m *ModelProviderService) listReadableModelProvidersForUserEE(ctx context.Context, tenantID, userID string) ([]*entity.TenantModelProvider, error) {
	return m.modelProviderDAO.ListByTenantIDAndUserID(ctx, dao.DB, tenantID, userID)
}

func (m *ModelProviderService) providersForInstanceList(ctx context.Context, providerIDOrName string) ([]*entity.TenantModelProvider, error) {
	provider, err := m.resolveProviderInTenant(ctx, providerIDOrName, permission.OperationRead)
	if err != nil {
		return nil, err
	}
	return []*entity.TenantModelProvider{provider}, nil
}

func queryScopeForAccessUser(ctx context.Context, checker *permission.Checker, query permission.ScopeQuery, _ string) (permission.QueryScope, error) {
	return checker.QueryScope(ctx, query)
}

// EnrichMessage leaves the heartbeat unchanged in the community edition.
func (h *AdminClient) EnrichMessage(message *common.BaseMessage) (*common.BaseMessage, error) {
	return message, nil
}

// UpdateClient ignores the heartbeat acknowledgement in the community edition.
func (h *AdminClient) UpdateClient(clientInfo any) error { return nil }

// prepareAgentCreation grants no additional permissions before an agent is
// created.
func prepareAgentCreation(ctx context.Context) (string, error) {
	return permission.UserIDFromContext(ctx), nil
}

// tenantMemberPermissionEE leaves membership checks to the community policy.
func tenantMemberPermissionEE(ctx context.Context, db *gorm.DB, tenantID string, required int32) (*entity.UserTenant, int32, bool, error) {
	return nil, 0, false, nil
}

func (*TenantService) listTenantMembersEE(context.Context, string) ([]*dao.TenantMemberItem, common.ErrorCode, bool, error) {
	return nil, common.CodeSuccess, false, nil
}

func markJoinedTenantMemberLeftEE(context.Context, *gorm.DB, string, string, *entity.UserTenant) (bool, error) {
	return false, nil
}

// recordTenantPermissionChangeEE records nothing: the community edition has no
// permission audit trail.
func recordTenantPermissionChangeEE(ctx context.Context, tx *gorm.DB, tenantID, eventType, targetType, targetID, resourceType, resourceID, actionType string, beforePermission, afterPermission int32, beforeData, afterData interface{}) error {
	return nil
}

// resolveTenantInviteRoleEE leaves invite roles to the community default.
func resolveTenantInviteRoleEE(*gorm.DB, string) (tenantInviteRole, bool, error) {
	return tenantInviteRole{}, false, nil
}

// checkTenantInviteEnabledEE leaves tenant invitation permission unrestricted
// in Community Edition, which does not configure Tenant system-role gates.
func (*TenantService) checkTenantInviteEnabledEE(context.Context, string) (common.ErrorCode, bool, error) {
	return common.CodeSuccess, false, nil
}

// resolveCurrentUserTenantEE leaves tenant resolution to the community path.
func resolveCurrentUserTenantEE(context.Context, string) (*entity.UserTenant, bool, error) {
	return nil, false, nil
}

// resolveDefaultTenantIDEE leaves tenant resolution to the community path.
func resolveDefaultTenantIDEE(context.Context) (string, bool, error) {
	return "", false, nil
}

// resolveUserAuthTenant discards tenant context because community credentials identify only a user.
func resolveUserAuthTenant(_ context.Context, _ *entity.User, _ string) (string, error) {
	return "", nil
}

// ResolveLoginTenantID leaves tenant selection to the community login path.
func ResolveLoginTenantID(_ context.Context, _ *entity.User) (string, error) {
	return "", nil
}

// defaultTierResolver keeps the community build on a single tier. All tenants
// share the default lane, which reuses the OSS community queue (TaskSubject), so
// an OSS split needs no rename and OSS and EE can share one NATS server. Tiering
// does not otherwise change community behavior.
func defaultTierResolver(_ context.Context, _ string) (common.Tier, error) {
	return common.TierDefault, nil
}

func logChatCompletion(_ context.Context, _ *entity.Chat, _ []map[string]any, _ bool, in <-chan AsyncChatResult) <-chan AsyncChatResult {
	return in
}

func prepareAgentOutput(ctx context.Context) (context.Context, error) { return ctx, ctx.Err() }

func filterAgentOutput(_ context.Context, answer, thinking string, reference map[string]any) (string, string, map[string]any) {
	return answer, thinking, reference
}

func (*ChatPipelineService) filterChatOutput(_ context.Context, _ *entity.Chat, _ bool, input <-chan AsyncChatResult) <-chan AsyncChatResult {
	return input
}

func filterAskOutput(_ context.Context, input <-chan AskDelta) <-chan AskDelta { return input }

func logAgentCompletion(_ context.Context, _, _ string, _ any, in <-chan canvas.RunEvent) <-chan canvas.RunEvent {
	return in
}

func withAgentEventLog(ctx context.Context, _, _, _, _ string, _ map[string]any) context.Context {
	return ctx
}

// ── mode 8 (Codex) ───────────────────────────────────────────────────────────
// The community edition ships no mode 8 engine: internal/codexagent, the MCP
// bridge wiring and the Codex pipeline live in the *_ee.go files. These stubs
// keep the shared dispatch compiling, and every mode-8 request degrades to the
// regular pipeline because codexConfigured always reports false.

// ── mode 8 (Codex) ───────────────────────────────────────────────────────────
// The community edition ships no mode 8 engine: internal/codexagent, the MCP
// bridge wiring and the Codex pipeline live in the *_ee.go files. These stubs
// keep the shared dispatch compiling, and every mode-8 request degrades to the
// regular pipeline because codexConfigured always reports false.

// SetCodexConfig ignores the mode 8 configuration in the community edition.
func SetCodexConfig(cfg *serverconfig.CodexConfig) {}

// InstallCodexKVStore is a no-op in the community edition.
func InstallCodexKVStore() {}

// InstallCodexWebSearchResolver is a no-op in the community edition.
func InstallCodexWebSearchResolver() {}

// codexConfigured reports whether mode 8 can run. The community edition has no
// Codex engine, so it is always false and the turn falls back to the pipeline.
func codexConfigured() bool { return false }

// cleanupCodexThread drops a session's Codex thread mapping. The community
// edition never creates one, so this is a no-op.
func cleanupCodexThread(ctx context.Context, sessionID string) {}

// codexResponsesCapable reports whether the dialog model speaks the Responses
// API. Unused in the community edition (mode 8 is never selected), but declared
// so the shared dispatch compiles.
func (s *ChatPipelineService) codexResponsesCapable(ctx context.Context, userID string, chat *entity.Chat) bool {
	return false
}

// codexAgent runs one mode 8 turn. The community edition never reaches it:
// codexConfigured() is false, so the dispatcher keeps the regular pipeline.
func (s *ChatPipelineService) codexAgent(
	ctx context.Context,
	userID string,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
	useWebSearch bool,
	quote bool,
) (<-chan AsyncChatResult, error) {
	return nil, fmt.Errorf("codex (mode 8) is not available in the community edition")
}

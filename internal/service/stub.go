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

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// InitHTTPClientEE is a no-op in the community edition.
func (h *AdminClient) InitHTTPClientEE() error { return nil }

// EnrichMessage leaves the heartbeat unchanged in the community edition.
func (h *AdminClient) EnrichMessage(message *common.BaseMessage) (*common.BaseMessage, error) {
	return message, nil
}

// UpdateClient ignores the heartbeat acknowledgement in the community edition.
func (h *AdminClient) UpdateClient(clientInfo any) error { return nil }

// prepareAgentCreation grants no additional permissions before an agent is
// created.
func prepareAgentCreation(_ context.Context, userID string) (string, error) { return userID, nil }

// tenantMemberPermissionEE leaves membership checks to the community policy.
func tenantMemberPermissionEE(ctx context.Context, db *gorm.DB, userID, tenantID string, required int32) (*entity.UserTenant, int32, bool, error) {
	return nil, 0, false, nil
}

// recordTenantPermissionChangeEE records nothing: the community edition has no
// permission audit trail.
func recordTenantPermissionChangeEE(tx *gorm.DB, tenantID, actorUserID, eventType, targetType, targetID, resourceType, resourceID, actionType string, beforePermission, afterPermission int32, beforeData, afterData interface{}) error {
	return nil
}

// resolveTenantInviteRoleEE leaves invite roles to the community default.
func resolveTenantInviteRoleEE(*gorm.DB, string) (tenantInviteRole, bool, error) {
	return tenantInviteRole{}, false, nil
}

// checkTenantInviteEnabledEE leaves the invite switch to the community
// default.
func (*TenantService) checkTenantInviteEnabledEE(context.Context, string) (common.ErrorCode, bool, error) {
	return common.CodeSuccess, false, nil
}

// resolveCurrentUserTenantEE leaves tenant resolution to the community path.
func resolveCurrentUserTenantEE(context.Context, string) (*entity.UserTenant, bool, error) {
	return nil, false, nil
}

// resolveDefaultTenantIDEE leaves tenant resolution to the community path.
func resolveDefaultTenantIDEE(context.Context, string) (string, bool, error) {
	return "", false, nil
}

// resolveUserAuthTenant keeps the token's tenant unchanged.
func resolveUserAuthTenant(_ context.Context, _ *entity.User, tenantID string) (string, error) {
	return tenantID, nil
}

// LoginChannel is one authentication channel advertised to the login page.
type LoginChannel struct {
	Channel     string `json:"channel"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}

// GetLoginChannels returns no channel: the community edition only offers
// password login.
func (s *UserService) GetLoginChannels() ([]*LoginChannel, common.ErrorCode, error) {
	return make([]*LoginChannel, 0), common.CodeSuccess, nil
}

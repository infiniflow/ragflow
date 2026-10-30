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

package service

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service/billing"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantService tenant service
type TenantService struct {
	userTenantDAO *dao.UserTenantDAO
	userDAO       *dao.UserDAO
	kbDAO         *dao.KnowledgebaseDAO
	docEngine     engine.DocEngine
}

type tenantInviteRole struct {
	value    string
	roleID   *int32
	roleName string
}

// tenantMemberPermission keeps community membership policy in shared code and
// lets Enterprise apply its Tenant permission grants through the EE hook.
func tenantMemberPermission(ctx context.Context, db *gorm.DB, userID, tenantID string, required int32) (*entity.UserTenant, int32, error) {
	membership, mask, handled, err := tenantMemberPermissionEE(ctx, db, userID, tenantID, required)
	if err != nil || handled {
		return membership, mask, err
	}

	requirement := permission.TenantMember
	if required != 0 {
		requirement = permission.TenantOwner
	}
	checker := permission.NewDatabaseChecker(db)
	if err := checker.CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, requirement); err != nil {
		return nil, 0, err
	}
	var communityMembership entity.UserTenant
	if err := db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid)).
		Take(&communityMembership).Error; err != nil {
		return nil, 0, err
	}
	return &communityMembership, 0, nil
}

func tenantRoleErrorCode(err error) common.ErrorCode {
	var coded *common.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}
	code, _ := permissionresponse.Normalize(err)
	return code
}

func recordTenantPermissionChange(tx *gorm.DB, tenantID, actorUserID, eventType, targetType, targetID, resourceType, resourceID, actionType string, beforePermission, afterPermission int32, beforeData, afterData interface{}) error {
	return recordTenantPermissionChangeEE(tx, tenantID, actorUserID, eventType, targetType, targetID, resourceType, resourceID, actionType, beforePermission, afterPermission, beforeData, afterData)
}

func resolveTenantInviteRole(tx *gorm.DB, tenantID string) (tenantInviteRole, error) {
	role, handled, err := resolveTenantInviteRoleEE(tx, tenantID)
	if err != nil || handled {
		return role, err
	}
	return tenantInviteRole{value: TenantRoleNormal}, nil
}

func (s *TenantService) checkTenantInviteEnabled(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	code, handled, err := s.checkTenantInviteEnabledEE(ctx, tenantID)
	if err != nil || handled {
		return code, err
	}
	return common.CodeSuccess, nil
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		userTenantDAO: dao.NewUserTenantDAO(),
		userDAO:       dao.NewUserDAO(),
		kbDAO:         dao.NewKnowledgebaseDAO(),
		docEngine:     engine.Get(),
	}
}

// TenantListItem tenant list item response
type TenantListItem struct {
	TenantID     string  `json:"tenant_id"`
	Role         string  `json:"role"`
	Nickname     string  `json:"nickname"`
	Email        string  `json:"email"`
	Avatar       string  `json:"avatar"`
	UpdateDate   string  `json:"update_date"`
	DeltaSeconds float64 `json:"delta_seconds"`
}

// GetTenantList get tenant list for a user
func (s *TenantService) GetTenantList(ctx context.Context, userID string) ([]*TenantListItem, error) {
	tenants, err := s.userTenantDAO.GetTenantsByUserID(ctx, dao.DB, userID)
	if err != nil {
		return nil, err
	}

	result := make([]*TenantListItem, len(tenants))

	for i, t := range tenants {
		// Parse update_date and calculate delta_seconds
		var deltaSeconds float64
		if t.UpdateDate != "" {
			deltaSeconds, err = common.DeltaSeconds(t.UpdateDate)
			if err != nil {
				return nil, err
			}
		}

		result[i] = &TenantListItem{
			TenantID:     t.TenantID,
			Role:         t.Role,
			Nickname:     t.Nickname,
			Email:        t.Email,
			Avatar:       t.Avatar,
			UpdateDate:   t.UpdateDate,
			DeltaSeconds: deltaSeconds,
		}
	}

	return result, nil
}

// CreateMetadataStore creates the metadata store for a tenant
func (s *TenantService) CreateMetadataStore(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	// Call document engine to create doc meta table
	err := s.docEngine.CreateMetadataStore(ctx, tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to create metadata table: %w", err)
	}

	return common.CodeSuccess, nil
}

// DeleteMetadataStore deletes the metadata store for a tenant
func (s *TenantService) DeleteMetadataStore(ctx context.Context, tenantID string) (common.ErrorCode, error) {
	// Call document engine to delete doc meta table
	err := s.docEngine.DropMetadataStore(ctx, tenantID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete doc meta table: %w", err)
	}

	return common.CodeSuccess, nil
}

// CreateDatasetTableRequest represents the request for creating a dataset table
type CreateDatasetTableRequest struct {
	KBID       string `json:"kb_id" binding:"required"`
	VectorSize int    `json:"vector_size" binding:"required"`
	ParserID   string `json:"parser_id,omitempty"`
}

// CreateChunkStoreResponse represents the response for creating a chunk store
type CreateChunkStoreResponse struct {
	KBID       string `json:"kb_id"`
	TableName  string `json:"table_name"`
	VectorSize int    `json:"vector_size"`
}

// CreateChunkStore creates a chunk store in the document engine for a knowledge base
func (s *TenantService) CreateChunkStore(ctx context.Context, req *CreateDatasetTableRequest) (*CreateChunkStoreResponse, common.ErrorCode, error) {
	if req == nil {
		return nil, common.CodeDataError, fmt.Errorf("request is required")
	}
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, req.KBID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("knowledge base not found: %s", req.KBID)
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", req.KBID, err)
	}

	// vector_size is required
	vecSize := req.VectorSize
	if vecSize <= 0 {
		return nil, common.CodeDataError, fmt.Errorf("vector_size must be positive")
	}

	// Build table name prefix: ragflow_<tenant_id>
	tableName := fmt.Sprintf("ragflow_%s", kb.TenantID)

	// The dataset language picks the fulltext analyzer on engines that fix it at
	// index creation, so it has to travel with the table, not the query.
	language := ""
	if kb.Language != nil {
		language = *kb.Language
	}

	// Call document engine to create table
	// Full table name will be built as "{tableName}_{kb_id}"
	err = s.docEngine.CreateChunkStore(ctx, tableName, req.KBID, vecSize, req.ParserID, language)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to create dataset: %w", err)
	}

	return &CreateChunkStoreResponse{
		KBID:       req.KBID,
		TableName:  tableName,
		VectorSize: vecSize,
	}, common.CodeSuccess, nil
}

// DeleteChunkStore deletes the chunk store in the document engine for a knowledge base
func (s *TenantService) DeleteChunkStore(ctx context.Context, kbID string) (common.ErrorCode, error) {
	// Get KB to find tenant_id for building table name
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, kbID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeDataError, fmt.Errorf("knowledge base not found: %s", kbID)
		}
		return common.CodeServerError, fmt.Errorf("failed to query knowledge base %s: %w", kbID, err)
	}

	// Call document engine to delete table
	err = s.docEngine.DropChunkStore(ctx, fmt.Sprintf("ragflow_%s", kb.TenantID), kbID)
	if err != nil {
		return common.CodeServerError, fmt.Errorf("failed to delete table: %w", err)
	}

	return common.CodeSuccess, nil
}

// Tenant member role constants.
const (
	TenantRoleOwner  = "owner"
	TenantRoleNormal = "normal"
	TenantRoleAdmin  = "admin"
)

// TenantMemberResponse is one entry in the member list response.
type TenantMemberResponse struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	Role            string  `json:"role"`
	Status          string  `json:"status"`
	Nickname        string  `json:"nickname"`
	Email           string  `json:"email"`
	Avatar          string  `json:"avatar"`
	IsAuthenticated bool    `json:"is_authenticated"`
	IsActive        string  `json:"is_active"`
	IsAnonymous     bool    `json:"is_anonymous"`
	IsSuperuser     bool    `json:"is_superuser"`
	UpdateDate      string  `json:"update_date"`
	DeltaSeconds    float64 `json:"delta_seconds"`
}

// ListMembers returns all non-owner members of tenantID.
func (s *TenantService) ListMembers(ctx context.Context, userID, tenantID string) ([]*TenantMemberResponse, common.ErrorCode, error) {
	if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
		return nil, tenantRoleErrorCode(err), err
	}
	rows, err := s.userTenantDAO.GetMembersByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	result := make([]*TenantMemberResponse, 0, len(rows))
	for _, r := range rows {
		delta, _ := common.DeltaSeconds(r.UpdateDate)
		result = append(result, &TenantMemberResponse{
			ID:              r.ID,
			UserID:          r.UserID,
			Role:            r.Role,
			Status:          r.Status,
			Nickname:        r.Nickname,
			Email:           r.Email,
			Avatar:          r.Avatar,
			IsAuthenticated: r.IsAuthenticated,
			IsActive:        r.IsActive,
			IsAnonymous:     r.IsAnonymous,
			IsSuperuser:     r.IsSuperuser,
			UpdateDate:      r.UpdateDate,
			DeltaSeconds:    delta,
		})
	}
	return result, common.CodeSuccess, nil
}

// AddMemberRequest holds the invite payload.
type AddMemberRequest struct {
	Email string `json:"email"`
}

// AddMemberResponse holds the new member's public data.
type AddMemberResponse struct {
	ID       string `json:"id"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

// AddMember invites a user (by email) to the tenant.
func (s *TenantService) AddMember(ctx context.Context, userID, tenantID string, req *AddMemberRequest) (*AddMemberResponse, common.ErrorCode, error) {
	if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
		return nil, tenantRoleErrorCode(err), err
	}
	if req == nil {
		return nil, common.CodeArgumentError, fmt.Errorf("request is required")
	}
	// Seat quota: inviting one member consumes one seat.
	if billingService := billing.Get(); billingService != nil && billingService.Enabled() {
		if quotaErr := billingService.CheckResources(ctx, tenantID, billing.ResourceDelta{Members: 1}); quotaErr != nil {
			var insufficient *billing.InsufficientResourceError
			if errors.As(quotaErr, &insufficient) {
				return nil, common.CodeBillingSeatsInsufficient, quotaErr
			}
			return nil, common.CodeServerError, quotaErr
		}
	}
	if strings.TrimSpace(req.Email) == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("email is required")
	}
	if code, err := s.checkTenantInviteEnabled(ctx, tenantID); err != nil {
		return nil, code, err
	}

	invitee, err := s.userDAO.GetByEmail(ctx, dao.DB, req.Email)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, fmt.Errorf("user not found")
		}
		return nil, common.CodeDataError, err
	}
	var membership entity.UserTenant
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing entity.UserTenant
		loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", invitee.ID, tenantID).Take(&existing).Error
		if loadErr != nil && !errors.Is(loadErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load existing Tenant membership: %w", loadErr)
		}
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			status := entity.TenantMembershipStatusInvited
			membership = entity.UserTenant{ID: common.GenerateUUID(), UserID: invitee.ID, TenantID: tenantID, InvitedBy: userID, Status: &status}
			if err := tx.Create(&membership).Error; err != nil {
				return fmt.Errorf("failed to create invitation: %w", err)
			}
			return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.invited", "user", invitee.ID, "tenant_membership", membership.ID, "invite", 0, 0, nil,
				map[string]interface{}{"user_id": invitee.ID, "email": invitee.Email, "role": membership.Role, "invited_by": userID, "status": status})
		}
		oldStatus := tenantMembershipStatus(existing.Status)
		switch oldStatus {
		case entity.TenantMembershipStatusJoined:
			if existing.Role == TenantRoleOwner {
				return common.NewCodedError(common.CodeDataError, "user is already the tenant owner")
			}
			return common.NewCodedError(common.CodeDataError, "user is already a member")
		case entity.TenantMembershipStatusInvited:
			return common.NewCodedError(common.CodeDataError, "user already has a pending invitation")
		case entity.TenantMembershipStatusRequested, entity.TenantMembershipStatusRejected, entity.TenantMembershipStatusLeft, entity.TenantMembershipStatusRemoved:
		default:
			return common.NewCodedError(common.CodeConflict, "Tenant membership has an unsupported status")
		}
		before := map[string]interface{}{"user_id": invitee.ID, "role": existing.Role, "status": oldStatus, "invited_by": existing.InvitedBy}
		status := entity.TenantMembershipStatusInvited
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{"role": "", "status": status, "invited_by": userID}).Error; err != nil {
			return fmt.Errorf("reinvite Tenant member: %w", err)
		}
		existing.Role, existing.InvitedBy, existing.Status = "", userID, &status
		membership = existing
		return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.invited", "user", invitee.ID, "tenant_membership", membership.ID, "invite", 0, 0, before,
			map[string]interface{}{"user_id": invitee.ID, "email": invitee.Email, "role": membership.Role, "invited_by": userID, "status": status})
	})
	if err != nil {
		return nil, tenantRoleErrorCode(err), err
	}

	return tenantMemberResponse(invitee), common.CodeSuccess, nil
}

// RemoveMember removes a user from the tenant. The owner may remove members,
// and a member may remove themselves. An invitee may decline their own pending
// invitation. The tenant owner cannot be removed.
func (s *TenantService) RemoveMember(ctx context.Context, userID, tenantID, targetUserID string) (common.ErrorCode, error) {
	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	if userID != targetUserID {
		if _, _, err := tenantMemberPermission(ctx, dao.DB.WithContext(ctx), userID, tenantID, entity.TenantPermissionManageMembers); err != nil {
			return tenantRoleErrorCode(err), err
		}
	}
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership entity.UserTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", targetUserID, tenantID).Take(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewCodedError(common.CodeDataError, "user has no Tenant membership or invitation")
			}
			return fmt.Errorf("load Tenant membership: %w", err)
		}
		oldStatus := tenantMembershipStatus(membership.Status)
		if oldStatus == entity.TenantMembershipStatusRemoved || oldStatus == entity.TenantMembershipStatusRejected || oldStatus == entity.TenantMembershipStatusLeft {
			return nil
		}
		if membership.Role == TenantRoleOwner {
			return common.NewCodedError(common.CodeArgumentError, "cannot remove or leave the tenant owner")
		}

		newStatus := entity.TenantMembershipStatusRemoved
		eventType, actionType := "tenant.member.removed", "remove"
		if userID == targetUserID {
			switch oldStatus {
			case entity.TenantMembershipStatusInvited:
				newStatus = entity.TenantMembershipStatusRejected
				eventType, actionType = "tenant.invitation.declined", "reject"
			case entity.TenantMembershipStatusJoined:
				newStatus = entity.TenantMembershipStatusLeft
				eventType, actionType = "tenant.member.left", "leave"
			default:
				return common.NewCodedError(common.CodeDataError, "user has no active Tenant membership or invitation")
			}
		} else if oldStatus == entity.TenantMembershipStatusInvited {
			eventType, actionType = "tenant.invitation.revoked", "revoke"
		}

		before := map[string]interface{}{
			"user_id": membership.UserID, "role": membership.Role,
			"invited_by": membership.InvitedBy, "status": oldStatus,
		}
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", membership.ID).
			Updates(map[string]interface{}{"role": "", "status": newStatus}).Error; err != nil {
			return fmt.Errorf("update Tenant membership status: %w", err)
		}
		after := map[string]interface{}{
			"user_id": membership.UserID, "role": "",
			"invited_by": membership.InvitedBy, "status": newStatus,
		}
		return recordTenantPermissionChange(tx, tenantID, userID, eventType, "user", targetUserID, "tenant_membership", membership.ID, actionType, 0, 0, before, after)
	})
	if err != nil {
		return tenantRoleErrorCode(err), err
	}
	return common.CodeSuccess, nil
}

// AcceptInvite assigns the Tenant default role when a pending invitation is accepted.
func (s *TenantService) AcceptInvite(ctx context.Context, userID, tenantID string) (common.ErrorCode, error) {
	// Seat quota is enforced again at acceptance time: several pending
	// invitations can each fit the remaining seat but not all of them.
	if billingService := billing.Get(); billingService != nil && billingService.Enabled() {
		if quotaErr := billingService.CheckResources(ctx, tenantID, billing.ResourceDelta{Members: 1}); quotaErr != nil {
			if _, ok := quotaErr.(*billing.InsufficientResourceError); ok {
				return common.CodeBillingSeatsInsufficient, quotaErr
			}
			return common.CodeServerError, quotaErr
		}
	}

	if s.userTenantDAO == nil {
		return common.CodeServerError, fmt.Errorf("userTenantDAO not initialized")
	}
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var membership entity.UserTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id = ?", userID, tenantID).Take(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.NewCodedError(common.CodeDataError, "no pending invitation found")
			}
			return fmt.Errorf("load pending Tenant invitation: %w", err)
		}
		if tenantMembershipStatus(membership.Status) != entity.TenantMembershipStatusInvited {
			return common.NewCodedError(common.CodeArgumentError, "no pending invitation to accept")
		}
		role, err := resolveTenantInviteRole(tx, tenantID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(role.value) == "" {
			return common.NewCodedError(common.CodeConflict, "Tenant default role is invalid")
		}
		status := entity.TenantMembershipStatusJoined
		if err := tx.Model(&entity.UserTenant{}).Where("id = ?", membership.ID).
			Updates(map[string]interface{}{"role": role.value, "status": status}).Error; err != nil {
			return fmt.Errorf("accept Tenant invitation: %w", err)
		}
		before := map[string]interface{}{"user_id": userID, "role": membership.Role, "status": tenantMembershipStatus(membership.Status)}
		after := map[string]interface{}{"user_id": userID, "role": role.value, "status": status}
		if role.roleID != nil {
			after["role_id"] = *role.roleID
		}
		if role.roleName != "" {
			after["role_name"] = role.roleName
		}
		return recordTenantPermissionChange(tx, tenantID, userID, "tenant.member.joined", "user", userID, "tenant_membership", membership.ID, "join", 0, 0, before, after)
	})
	if err != nil {
		return tenantRoleErrorCode(err), err
	}
	return common.CodeSuccess, nil
}

func tenantMembershipStatus(status *string) string {
	if status == nil {
		return ""
	}
	return *status
}

func tenantMemberResponse(user *entity.User) *AddMemberResponse {
	avatar := ""
	if user.Avatar != nil {
		avatar = *user.Avatar
	}
	return &AddMemberResponse{ID: user.ID, Avatar: avatar, Email: user.Email, Nickname: user.Nickname}
}

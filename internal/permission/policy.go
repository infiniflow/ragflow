//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package permission

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

type communityPolicy struct {
	source Source
}

type membershipResult struct {
	membership *Membership
	err        error
}

type evaluation struct {
	source      Source
	memberships map[string]membershipResult
	resources   map[ResourceRef]Resource
	resourceErr map[ResourceRef]error
}

func newPolicy(source Source) policy {
	if activePolicy, handled := newPolicyEE(source); handled {
		return activePolicy
	}
	return &communityPolicy{source: source}
}

func (p *communityPolicy) CheckFeature(ctx context.Context, feature Feature, action Action) error {
	if strings.TrimSpace(UserIDFromContext(ctx)) == "" {
		return ErrUnauthenticated
	}
	if strings.TrimSpace(string(feature)) == "" || !validAction(action) {
		return ErrInvalidPermission
	}
	// Community edition has no role-based feature restrictions.
	return nil
}

func (p *communityPolicy) CheckTenant(ctx context.Context, tenantID string, requirement TenantRequirement) error {
	return newEvaluation(p.source).checkTenant(ctx, tenantID, requirement)
}

func (p *communityPolicy) CheckResource(ctx context.Context, ref ResourceRef, operation Operation) error {
	if strings.TrimSpace(UserIDFromContext(ctx)) == "" {
		return ErrUnauthenticated
	}
	return newEvaluation(p.source).checkResource(ctx, ref, operation)
}

func (p *communityPolicy) CheckResourcePermission(ctx context.Context, ref ResourceRef, required int32) error {
	if strings.TrimSpace(UserIDFromContext(ctx)) == "" {
		return ErrUnauthenticated
	}
	if !validResourcePermission(required) {
		return ErrInvalidPermission
	}
	access, err := newEvaluation(p.source).resolveAccess(ctx, ref)
	if err != nil {
		return err
	}
	if !resourcePermissionAllows(access.PermissionMask, required) {
		return ErrPermissionDenied
	}
	return nil
}

func (p *communityPolicy) CheckDependency(ctx context.Context, entryRef, dependencyRef ResourceRef, entryOperation, dependencyOperation Operation) error {
	if strings.TrimSpace(UserIDFromContext(ctx)) == "" {
		return ErrUnauthenticated
	}
	if !validOperation(entryOperation) || !validOperation(dependencyOperation) {
		return ErrInvalidPermission
	}
	evaluation := newEvaluation(p.source)
	return evaluation.checkDependency(ctx, entryRef, dependencyRef, entryOperation, dependencyOperation)
}

func (p *communityPolicy) ResolveAccess(ctx context.Context, ref ResourceRef) (Access, error) {
	return newEvaluation(p.source).resolveAccess(ctx, ref)
}

func (p *communityPolicy) Scope(ctx context.Context, query ScopeQuery) (Scope, error) {
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if userID == "" {
		return Scope{}, ErrUnauthenticated
	}
	if strings.TrimSpace(string(query.Kind)) == "" || !validOperation(query.Operation) {
		return Scope{}, ErrInvalidPermission
	}
	if (query.Parent.ID == "") != (query.Parent.Kind == "") {
		return Scope{}, ErrInvalidPermission
	}
	if query.Entry != nil && (!validResourceRef(*query.Entry) || !validOperation(query.EntryOp)) {
		return Scope{}, ErrInvalidPermission
	}

	evaluation := newEvaluation(p.source)
	candidateRefs, err := p.source.ListResourceRefs(ctx, query.Kind, query.Parent)
	if err != nil {
		return Scope{}, fmt.Errorf("permission: list resource candidates: %w", err)
	}
	candidateRefs = uniqueRefs(candidateRefs)
	resourcesToLoad := append([]ResourceRef(nil), candidateRefs...)
	if query.Parent.ID != "" {
		resourcesToLoad = append(resourcesToLoad, query.Parent)
	}
	if query.Entry != nil {
		resourcesToLoad = append(resourcesToLoad, *query.Entry)
	}
	if err := evaluation.loadResources(ctx, resourcesToLoad); err != nil {
		return Scope{}, err
	}

	var entryRef ResourceRef
	if query.Entry != nil {
		entryRef = *query.Entry
	}
	accessible := make([]ResourceRef, 0, len(candidateRefs))
	for _, ref := range candidateRefs {
		var checkErr error
		if query.Entry == nil {
			checkErr = evaluation.checkResource(ctx, ref, query.Operation)
		} else {
			checkErr = evaluation.checkDependency(ctx, entryRef, ref, query.EntryOp, query.Operation)
		}
		if checkErr == nil {
			accessible = append(accessible, ref)
			continue
		}
		if errorsIsPermissionDenial(checkErr) || errors.Is(checkErr, ErrResourceNotFound) {
			continue
		}
		return Scope{}, checkErr
	}

	// In the community edition the effective tenant comes from the persisted
	// resource boundary, not from a caller-selected tenant. Scopes without a
	// single parent or entry can contain resources from multiple tenants, so
	// their explicit authorized IDs are the boundary and TenantID stays empty.
	var tenantID string
	if query.Parent.ID != "" {
		if parent, parentErr := evaluation.getResource(query.Parent); parentErr == nil {
			tenantID = parent.TenantID
		}
	}
	if query.Entry != nil {
		if entry, entryErr := evaluation.getResource(*query.Entry); entryErr == nil {
			tenantID = entry.TenantID
		}
	}
	scope := Scope{TenantID: tenantID, Kind: query.Kind, Parent: query.Parent, Operation: query.Operation}
	if len(accessible) == 0 {
		scope.Mode = ScopeNone
		return scope, nil
	}
	scope.Mode = ScopeIDs
	scope.ResourceIDs = make([]string, 0, len(accessible))
	for _, ref := range accessible {
		scope.ResourceIDs = append(scope.ResourceIDs, ref.ID)
	}
	return scope, nil
}

func (p *communityPolicy) QueryScope(ctx context.Context, query ScopeQuery) (QueryScope, bool, error) {
	if query.Kind == ResourceKindModelProvider || query.Kind == ResourceKindModelInstance || query.Kind == ResourceKindModel {
		return p.modelQueryScope(ctx, query)
	}
	if query.Kind == ResourceKindMCPServer && query.Operation == OperationRead && query.Parent.ID == "" && query.Parent.Kind == "" && query.Entry == nil {
		return p.mcpServerReadQueryScope(ctx, query.TenantID)
	}
	if query.Kind == ResourceKindDocument && query.Parent.Kind == ResourceKindDataset && strings.TrimSpace(query.Parent.ID) != "" && query.Entry == nil {
		if err := p.CheckResource(ctx, query.Parent, OperationRead); err != nil {
			return QueryScope{}, true, err
		}
		return QueryScope{}, false, nil
	}
	if query.Kind != ResourceKindDataset || query.Operation != OperationRead || query.Parent.ID != "" || query.Parent.Kind != "" || query.Entry != nil {
		return QueryScope{}, false, nil
	}
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if userID == "" {
		return QueryScope{}, true, ErrUnauthenticated
	}
	source, ok := p.source.(*databaseSource)
	if !ok || source.db == nil || tenantRoleMembershipEnabled(source.db) {
		return QueryScope{}, false, nil
	}
	return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
		member := source.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Select("1").
			Where("user_tenant.tenant_id = knowledgebase.tenant_id AND user_tenant.user_id = ? AND user_tenant.role = ? AND user_tenant.status = ?",
				userID, string(RoleNormal), string(entity.StatusValid))
		owner := "(TRIM(knowledgebase.user_id) = ? OR (NULLIF(TRIM(knowledgebase.user_id), '') IS NULL AND TRIM(knowledgebase.created_by) = ?))"
		return query.
			Where("knowledgebase.status = ?", string(entity.StatusValid)).
			Where("("+owner+" OR (knowledgebase.permission = ? AND EXISTS (?)))",
				userID, userID, string(entity.TenantPermissionTeam), member)
	}}, true, nil
}

func (p *communityPolicy) modelQueryScope(ctx context.Context, query ScopeQuery) (QueryScope, bool, error) {
	parent := query.Parent
	if query.Entry != nil || (parent.ID == "") != (parent.Kind == "") {
		return QueryScope{}, false, nil
	}
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if userID == "" {
		return QueryScope{}, true, ErrUnauthenticated
	}
	source, ok := p.source.(*databaseSource)
	if !ok || source.db == nil || tenantRoleMembershipEnabled(source.db) {
		return QueryScope{}, false, nil
	}
	if query.Kind == ResourceKindModelInstance && parent.ID != "" && parent.Kind != ResourceKindModelProvider {
		return QueryScope{}, false, nil
	}
	if query.Kind == ResourceKindModel && parent.ID != "" && parent.Kind != ResourceKindModelProvider && parent.Kind != ResourceKindModelInstance {
		return QueryScope{}, false, nil
	}

	providerAccess := func(providerIDColumn, parentID string, ownerOnly bool) *gorm.DB {
		provider := source.db.WithContext(ctx).Model(&entity.TenantModelProvider{}).
			Select("1").
			Where("tenant_model_provider.id = " + providerIDColumn)
		if parentID != "" {
			provider = provider.Where("tenant_model_provider.id = ?", parentID)
		}
		owner := source.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Select("user_tenant.user_id").
			Where("user_tenant.tenant_id = tenant_model_provider.tenant_id AND user_tenant.role = ? AND user_tenant.status = ?", string(RoleOwner), string(entity.StatusValid)).
			Limit(1)
		provider = provider.Where("tenant_model_provider.user_id = COALESCE((?), tenant_model_provider.tenant_id)", owner)
		if ownerOnly {
			return provider.Where("tenant_model_provider.user_id = ?", userID)
		}
		membership := source.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Select("1").
			Where("user_tenant.tenant_id = tenant_model_provider.tenant_id AND user_tenant.user_id = ? AND user_tenant.status = ? AND user_tenant.role IN ?",
				userID, string(entity.StatusValid), []string{string(RoleOwner), string(RoleAdmin), string(RoleNormal)})
		return provider.Where("(tenant_model_provider.user_id = ? OR EXISTS (?))", userID, membership)
	}

	switch query.Kind {
	case ResourceKindModelProvider:
		if parent.ID != "" || query.Operation != OperationRead {
			return QueryScope{}, false, nil
		}
		return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
			db := source.db.WithContext(ctx)
			owner := db.Model(&entity.UserTenant{}).
				Select("user_tenant.user_id").
				Where("user_tenant.tenant_id = tenant_model_provider.tenant_id AND user_tenant.role = ? AND user_tenant.status = ?", string(RoleOwner), string(entity.StatusValid)).
				Limit(1)
			membership := db.Model(&entity.UserTenant{}).
				Select("1").
				Where("user_tenant.tenant_id = tenant_model_provider.tenant_id AND user_tenant.user_id = ? AND user_tenant.status = ? AND user_tenant.role IN ?",
					userID, string(entity.StatusValid), []string{string(RoleOwner), string(RoleAdmin), string(RoleNormal)})
			return query.
				Where("tenant_model_provider.user_id = COALESCE((?), tenant_model_provider.tenant_id)", owner).
				Where("(tenant_model_provider.user_id = ? OR EXISTS (?))", userID, membership)
		}}, true, nil
	case ResourceKindModelInstance:
		if query.Operation != OperationRead && query.Operation != OperationUse {
			return QueryScope{}, false, nil
		}
		ownerOnly := query.Operation == OperationRead
		return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
			return query.Where("EXISTS (?)", providerAccess("tenant_model_instance.provider_id", parent.ID, ownerOnly))
		}}, true, nil
	case ResourceKindModel:
		if query.Operation != OperationRead && query.Operation != OperationUse {
			return QueryScope{}, false, nil
		}
		return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
			modelQuery := query.Where("EXISTS (?)", providerAccess("tenant_model.provider_id", "", false))
			switch parent.Kind {
			case ResourceKindModelProvider:
				modelQuery = modelQuery.Where("tenant_model.provider_id = ?", parent.ID)
			case ResourceKindModelInstance:
				modelQuery = modelQuery.Where("tenant_model.instance_id = ?", parent.ID)
			}
			return modelQuery
		}}, true, nil
	default:
		return QueryScope{}, false, nil
	}
}

func (p *communityPolicy) mcpServerReadQueryScope(ctx context.Context, targetTenantID string) (QueryScope, bool, error) {
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if userID == "" {
		return QueryScope{}, true, ErrUnauthenticated
	}
	tenantID := strings.TrimSpace(targetTenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(TenantIDFromContext(ctx))
	}
	if tenantID == "" {
		return QueryScope{}, true, ErrInvalidPermission
	}
	source, ok := p.source.(*databaseSource)
	if !ok || source.db == nil {
		return QueryScope{}, false, nil
	}
	return QueryScope{apply: func(query *gorm.DB) *gorm.DB {
		activeOwner := source.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Select("1").
			Where("user_tenant.tenant_id = mcp_server.tenant_id AND user_tenant.user_id = ? AND user_tenant.role = ? AND user_tenant.status = ?",
				userID, string(RoleOwner), string(entity.StatusValid))
		return query.Where("mcp_server.tenant_id = ?", tenantID).Where("EXISTS (?)", activeOwner)
	}}, true, nil
}

func (p *communityPolicy) FilterResources(ctx context.Context, refs []ResourceRef, operation Operation) ([]ResourceRef, error) {
	accesses, err := p.FilterAccesses(ctx, refs, operation)
	if err != nil {
		return nil, err
	}
	accessible := make([]ResourceRef, 0, len(accesses))
	for _, ref := range uniqueRefs(refs) {
		if _, ok := accesses[ref]; ok {
			accessible = append(accessible, ref)
		}
	}
	return accessible, nil
}

func (p *communityPolicy) ResolveAccesses(ctx context.Context, refs []ResourceRef) (map[ResourceRef]Access, error) {
	if strings.TrimSpace(UserIDFromContext(ctx)) == "" {
		return nil, ErrUnauthenticated
	}
	evaluation := newEvaluation(p.source)
	refs = uniqueRefs(refs)
	if err := evaluation.loadResources(ctx, refs); err != nil {
		return nil, err
	}
	accesses := make(map[ResourceRef]Access, len(refs))
	for _, ref := range refs {
		access, err := evaluation.resolveAccess(ctx, ref)
		if err == nil {
			accesses[ref] = access
			continue
		}
		if errorsIsPermissionDenial(err) || errors.Is(err, ErrResourceNotFound) {
			continue
		}
		return nil, err
	}
	return accesses, nil
}

func (p *communityPolicy) FilterAccesses(ctx context.Context, refs []ResourceRef, operation Operation) (map[ResourceRef]Access, error) {
	if !validOperation(operation) {
		return nil, ErrInvalidPermission
	}
	accesses, err := p.ResolveAccesses(ctx, refs)
	if err != nil {
		return nil, err
	}
	for ref, access := range accesses {
		if !access.Allows(operation) {
			delete(accesses, ref)
		}
	}
	return accesses, nil
}

func newEvaluation(source Source) *evaluation {
	return &evaluation{
		source:      source,
		memberships: make(map[string]membershipResult),
		resources:   make(map[ResourceRef]Resource),
		resourceErr: make(map[ResourceRef]error),
	}
}

func (e *evaluation) checkTenant(ctx context.Context, tenantID string, requirement TenantRequirement) error {
	return e.checkTenantForUser(ctx, UserIDFromContext(ctx), tenantID, requirement)
}

func (e *evaluation) checkTenantForUser(ctx context.Context, userID, tenantID string, requirement TenantRequirement) error {
	if strings.TrimSpace(userID) == "" {
		return ErrUnauthenticated
	}
	if strings.TrimSpace(tenantID) == "" || !validTenantRequirement(requirement) {
		return ErrInvalidPermission
	}
	membership, err := e.getMembership(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if !membershipSatisfies(membership, requirement) {
		return ErrMembershipNotFound
	}
	return nil
}

func (e *evaluation) checkResource(ctx context.Context, ref ResourceRef, operation Operation) error {
	if !validOperation(operation) {
		return ErrInvalidPermission
	}
	access, err := e.resolveAccess(ctx, ref)
	if err != nil {
		return err
	}
	if !accessAllows(access, operation) {
		return ErrPermissionDenied
	}
	return nil
}

func (e *evaluation) checkDependency(ctx context.Context, entryRef, dependencyRef ResourceRef, entryOperation, dependencyOperation Operation) error {
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if strings.TrimSpace(userID) == "" {
		return ErrUnauthenticated
	}
	if !validOperation(entryOperation) || !validOperation(dependencyOperation) {
		return ErrInvalidPermission
	}
	if err := e.loadResources(ctx, []ResourceRef{entryRef, dependencyRef}); err != nil {
		return err
	}
	entry, err := e.getResource(entryRef)
	if err != nil {
		return err
	}
	entryAccess, err := e.resolveResource(ctx, entry)
	if err != nil {
		return err
	}
	if !accessAllows(entryAccess, entryOperation) {
		return ErrPermissionDenied
	}
	dependency, err := e.getResource(dependencyRef)
	if err != nil {
		return err
	}
	userAccess, err := e.resolveResource(ctx, dependency)
	if err != nil {
		return err
	}
	if !accessAllows(userAccess, dependencyOperation) {
		return ErrPermissionDenied
	}
	ownerAccess, err := e.resolveResourceForUser(ctx, entry.OwnerUserID, dependency)
	if err != nil {
		if errorsIsPermissionDenial(err) {
			return ErrPermissionDenied
		}
		return err
	}
	if !accessAllows(ownerAccess, dependencyOperation) {
		return ErrPermissionDenied
	}
	return nil
}

func (e *evaluation) resolveAccess(ctx context.Context, ref ResourceRef) (Access, error) {
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if strings.TrimSpace(userID) == "" {
		return Access{}, ErrUnauthenticated
	}
	if err := e.loadResources(ctx, []ResourceRef{ref}); err != nil {
		return Access{}, err
	}
	resource, err := e.getResource(ref)
	if err != nil {
		return Access{}, err
	}
	return e.resolveResource(ctx, resource)
}

func (e *evaluation) resolveResource(ctx context.Context, resource Resource) (Access, error) {
	userID := strings.TrimSpace(UserIDFromContext(ctx))
	if userID == "" {
		return Access{}, ErrUnauthenticated
	}
	return e.resolveResourceForUser(ctx, userID, resource)
}

func (e *evaluation) resolveResourceForUser(ctx context.Context, userID string, resource Resource) (Access, error) {
	if resource.TenantID == "" || resource.OwnerUserID == "" || !validResourceRef(resource.Ref) {
		return Access{}, fmt.Errorf("%w: incomplete resource authorization facts", ErrInvalidPermission)
	}
	if !resource.Active {
		return Access{}, ErrResourceNotFound
	}
	if !validOperations(resource.OwnerOperations) {
		return Access{}, fmt.Errorf("%w: invalid owner operations", ErrInvalidPermission)
	}
	if !validOperations(resource.TenantOperations) || !validOperations(resource.CreatorOperations) {
		return Access{}, fmt.Errorf("%w: invalid delegated operations", ErrInvalidPermission)
	}
	switch resource.Visibility {
	case VisibilityPrivate, VisibilityTenant, VisibilityShared:
	default:
		return Access{}, fmt.Errorf("%w: unknown resource visibility %q", ErrInvalidPermission, resource.Visibility)
	}
	if userID == resource.OwnerUserID {
		return resourceAccess(resource, AccessSourceOwner), nil
	}

	switch resource.Visibility {
	case VisibilityPrivate:
		return Access{}, ErrPermissionDenied
	case VisibilityTenant:
		requirement := resource.TenantRequirement
		if requirement == "" {
			requirement = TenantNormalMember
		}
		if err := e.checkTenantForUser(ctx, userID, resource.TenantID, requirement); err != nil {
			if errors.Is(err, ErrMembershipNotFound) {
				return Access{}, ErrPermissionDenied
			}
			return Access{}, err
		}
		if userID == resource.CreatedBy && resource.CreatorOperations != nil {
			return resourceAccessWithOperations(resource, AccessSourceCreator, resource.CreatorOperations), nil
		}
		operations := resource.TenantOperations
		if operations == nil {
			operations = resource.OwnerOperations
		}
		return resourceAccessWithOperations(resource, AccessSourceTenant, operations), nil
	case VisibilityShared:
		if slices.Contains(resource.SharedWithUserIDs, userID) {
			return resourceAccess(resource, AccessSourceShared), nil
		}
		for _, tenantID := range resource.SharedWithTenantIDs {
			if err := e.checkTenantForUser(ctx, userID, tenantID, TenantNormalMember); err != nil {
				if errors.Is(err, ErrMembershipNotFound) {
					continue
				}
				return Access{}, err
			}
			return resourceAccess(resource, AccessSourceShared), nil
		}
		return Access{}, ErrPermissionDenied
	default:
		return Access{}, fmt.Errorf("%w: unknown resource visibility %q", ErrInvalidPermission, resource.Visibility)
	}
}

func (e *evaluation) loadResources(ctx context.Context, refs []ResourceRef) error {
	if e.source == nil {
		return ErrSourceUnavailable
	}
	var missing []ResourceRef
	for _, ref := range uniqueRefs(refs) {
		if _, ok := e.resources[ref]; ok {
			continue
		}
		if _, ok := e.resourceErr[ref]; ok {
			continue
		}
		if !validResourceRef(ref) {
			return ErrInvalidPermission
		}
		missing = append(missing, ref)
	}
	if len(missing) == 0 {
		return nil
	}
	resources, err := e.source.GetResources(ctx, missing)
	if err != nil {
		return fmt.Errorf("permission: load resources: %w", err)
	}
	for _, ref := range missing {
		e.resourceErr[ref] = ErrResourceNotFound
	}
	for _, resource := range resources {
		if _, requested := e.resourceErr[resource.Ref]; requested {
			e.resources[resource.Ref] = resource
			delete(e.resourceErr, resource.Ref)
		}
	}
	return nil
}

func (e *evaluation) getResource(ref ResourceRef) (Resource, error) {
	if !validResourceRef(ref) {
		return Resource{}, ErrInvalidPermission
	}
	if resource, ok := e.resources[ref]; ok {
		return resource, nil
	}
	if err, ok := e.resourceErr[ref]; ok {
		return Resource{}, err
	}
	return Resource{}, ErrResourceNotFound
}

func (e *evaluation) getMembership(ctx context.Context, userID, tenantID string) (*Membership, error) {
	if e.source == nil {
		return nil, ErrSourceUnavailable
	}
	key := userID + "\x00" + tenantID
	if result, ok := e.memberships[key]; ok {
		return result.membership, result.err
	}
	membership, err := e.source.GetMembership(ctx, userID, tenantID)
	if err != nil {
		err = fmt.Errorf("permission: load tenant membership: %w", err)
	} else if membership == nil || !membership.Active || membership.UserID != userID || membership.TenantID != tenantID {
		membership = nil
		err = ErrMembershipNotFound
	}
	e.memberships[key] = membershipResult{membership: membership, err: err}
	return membership, err
}

func errorsIsPermissionDenial(err error) bool {
	return errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrMembershipNotFound)
}

func validResourceRef(ref ResourceRef) bool {
	return strings.TrimSpace(string(ref.Kind)) != "" && strings.TrimSpace(ref.ID) != ""
}

func validAction(action Action) bool {
	switch action {
	case ActionEnable, ActionRead, ActionWrite, ActionShare:
		return true
	default:
		return false
	}
}

func validOperation(operation Operation) bool {
	switch operation {
	case OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationDeleteContent, OperationShare, OperationRun, OperationUse:
		return true
	default:
		return false
	}
}

func validResourcePermission(required int32) bool {
	return required == ResourcePermissionRead || required == ResourcePermissionWrite || required == ResourcePermissionManage
}

func resourcePermissionAllows(granted, required int32) bool {
	if granted&ResourcePermissionOwner != 0 {
		return true
	}
	var sufficient int32
	switch required {
	case ResourcePermissionRead:
		sufficient = ResourcePermissionRead | ResourcePermissionWrite | ResourcePermissionManage
	case ResourcePermissionWrite:
		sufficient = ResourcePermissionWrite | ResourcePermissionManage
	case ResourcePermissionManage:
		sufficient = ResourcePermissionManage
	default:
		return false
	}
	return granted&sufficient != 0
}

func validTenantRequirement(requirement TenantRequirement) bool {
	switch requirement {
	case TenantMember, TenantNormalMember, TenantAdmin, TenantOwner:
		return true
	default:
		return false
	}
}

func membershipSatisfies(membership *Membership, requirement TenantRequirement) bool {
	if membership == nil || !membership.Active {
		return false
	}
	switch requirement {
	case TenantMember:
		return membership.Role == RoleOwner || membership.Role == RoleAdmin || membership.Role == RoleNormal
	case TenantNormalMember:
		return membership.Role == RoleNormal
	case TenantAdmin:
		return membership.Role == RoleOwner || membership.Role == RoleAdmin
	case TenantOwner:
		return membership.Role == RoleOwner
	default:
		return false
	}
}

func validOperations(operations []Operation) bool {
	for _, operation := range operations {
		if !validOperation(operation) {
			return false
		}
	}
	return true
}

func resourceAccess(resource Resource, source AccessSource) Access {
	return resourceAccessWithOperations(resource, source, resource.OwnerOperations)
}

func resourceAccessWithOperations(resource Resource, source AccessSource, operations []Operation) Access {
	permissionMask := int32(0)
	for _, operation := range operations {
		switch operation {
		case OperationRead, OperationRun, OperationUse:
			permissionMask |= ResourcePermissionRead
		case OperationCreate, OperationUpdate, OperationDeleteContent:
			permissionMask |= ResourcePermissionWrite
		case OperationDelete, OperationShare:
			permissionMask |= ResourcePermissionManage
		}
	}
	if source == AccessSourceOwner {
		permissionMask = ResourcePermissionOwner
	}
	return Access{
		Resource:       resource.Ref,
		TenantID:       resource.TenantID,
		Source:         source,
		PermissionMask: permissionMask,
		Operations:     append([]Operation(nil), operations...),
	}
}

func accessAllows(access Access, operation Operation) bool {
	return access.Allows(operation)
}

func uniqueRefs(refs []ResourceRef) []ResourceRef {
	seen := make(map[ResourceRef]struct{}, len(refs))
	unique := make([]ResourceRef, 0, len(refs))
	for _, ref := range refs {
		if !validResourceRef(ref) {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		unique = append(unique, ref)
	}
	return unique
}

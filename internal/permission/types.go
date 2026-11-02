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

// Package permission provides shared permission contracts and edition policies.
package permission

import (
	"context"
	"errors"
)

type subjectContextKey struct{}

var (
	ErrUnauthenticated    = errors.New("permission: user is required")
	ErrInvalidSubject     = errors.New("permission: tenant is required")
	ErrResourceNotFound   = errors.New("permission: resource not found")
	ErrMembershipNotFound = errors.New("permission: active tenant membership not found")
	ErrPermissionDenied   = errors.New("permission: access denied")
	ErrInvalidPermission  = errors.New("permission: invalid permission request")
	ErrSourceUnavailable  = errors.New("permission: source is not configured")
)

// Subject identifies the authenticated user and, when the policy requires it,
// the active tenant context. Community-edition resource checks derive the
// effective tenant from the persisted resource, so TenantID may be empty.
type Subject struct {
	UserID   string
	TenantID string
}

// WithSubject attaches the authenticated user and active Tenant to a request
// context so shared service helpers can use the same authorization subject.
func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectContextKey{}, subject)
}

// SubjectFromContext returns the authenticated permission subject attached to
// the request context, if one is present.
func SubjectFromContext(ctx context.Context) (Subject, bool) {
	if ctx == nil {
		return Subject{}, false
	}
	subject, ok := ctx.Value(subjectContextKey{}).(Subject)
	return subject, ok
}

func resolveSubject(ctx context.Context, subject Subject) (Subject, error) {
	requestSubject, ok := SubjectFromContext(ctx)
	if !ok {
		return subject, nil
	}
	if subject.UserID == "" {
		subject.UserID = requestSubject.UserID
	} else if requestSubject.UserID != "" && subject.UserID != requestSubject.UserID {
		return Subject{}, ErrInvalidSubject
	}
	if subject.TenantID == "" {
		subject.TenantID = requestSubject.TenantID
	} else if requestSubject.TenantID != "" && subject.TenantID != requestSubject.TenantID {
		return Subject{}, ErrInvalidSubject
	}
	return subject, nil
}

// ResourceKind identifies a resource family without coupling this package to business entities.
type ResourceKind string

const (
	ResourceKindDataset       ResourceKind = "dataset"
	ResourceKindDocument      ResourceKind = "document"
	ResourceKindCanvas        ResourceKind = "canvas"
	ResourceKindAgentSession  ResourceKind = "agent_session"
	ResourceKindMemory        ResourceKind = "memory"
	ResourceKindFile          ResourceKind = "file"
	ResourceKindFolder        ResourceKind = "folder"
	ResourceKindSearchApp     ResourceKind = "search_app"
	ResourceKindChat          ResourceKind = "chat"
	ResourceKindChatSession   ResourceKind = "chat_session"
	ResourceKindChatChannel   ResourceKind = "chat_channel"
	ResourceKindConnector     ResourceKind = "connector"
	ResourceKindSyncTask      ResourceKind = "sync_task"
	ResourceKindModelProvider ResourceKind = "model_provider"
	ResourceKindModelInstance ResourceKind = "model_instance"
	ResourceKindModel         ResourceKind = "model"
	ResourceKindMCPServer     ResourceKind = "mcp_server"
	ResourceKindSkillSpace    ResourceKind = "skill_space"
)

// ResourceRef identifies a persisted resource.
type ResourceRef struct {
	Kind ResourceKind
	ID   string
}

// Feature identifies a feature whose availability is edition-dependent.
type Feature string

// Action identifies a feature-level action.
type Action string

const (
	ActionEnable Action = "enable"
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionShare  Action = "share"
)

// Operation identifies an operation on a concrete resource.
type Operation string

// Resource permission masks describe direct resource grants independently of
// feature-role limits. The permission UI uses the same bit layout as the
// resource grant table.
const (
	ResourcePermissionRead int32 = 1 << iota
	ResourcePermissionWrite
	ResourcePermissionManage
	ResourcePermissionOwner
)

const (
	OperationRead          Operation = "read"
	OperationCreate        Operation = "create"
	OperationUpdate        Operation = "update"
	OperationDelete        Operation = "delete"
	OperationDeleteContent Operation = "delete_content"
	OperationShare         Operation = "share"
	OperationRun           Operation = "run"
	OperationUse           Operation = "use"
)

// TenantRequirement describes the tenant role required by a check.
type TenantRequirement string

const (
	TenantMember       TenantRequirement = "member"
	TenantNormalMember TenantRequirement = "normal_member"
	TenantAdmin        TenantRequirement = "admin"
	TenantOwner        TenantRequirement = "owner"
)

// TenantRole is the normalized role in a user-tenant membership.
type TenantRole string

const (
	RoleOwner  TenantRole = "owner"
	RoleAdmin  TenantRole = "admin"
	RoleNormal TenantRole = "normal"
)

// Membership is the normalized active membership fact returned by Source.
type Membership struct {
	ID       string
	UserID   string
	TenantID string
	Role     TenantRole
	Active   bool
}

// Visibility describes how a resource is exposed in the community edition.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTenant  Visibility = "tenant"
	VisibilityShared  Visibility = "shared"
)

// Resource contains normalized authorization and attribution facts loaded from
// the persisted business resource. TenantID, CreatedBy, and OwnerUserID are
// independent facts: the resource tenant, creator, and permission owner need
// not be the same identity.
type Resource struct {
	Ref               ResourceRef
	TenantID          string
	CreatedBy         string
	OwnerUserID       string
	Visibility        Visibility
	TenantRequirement TenantRequirement
	OwnerOperations   []Operation
	// TenantOperations overrides OwnerOperations for tenant members when set.
	TenantOperations []Operation
	// CreatorOperations grants extra operations to CreatedBy after tenant access is verified.
	CreatorOperations   []Operation
	SharedWithUserIDs   []string
	SharedWithTenantIDs []string
	Active              bool
}

// AccessSource identifies the rule that granted access.
type AccessSource string

const (
	AccessSourceOwner   AccessSource = "owner"
	AccessSourceTenant  AccessSource = "tenant"
	AccessSourceCreator AccessSource = "creator"
	AccessSourceShared  AccessSource = "shared"
)

// Access describes the operations a subject can perform on a resource.
type Access struct {
	Resource       ResourceRef
	TenantID       string
	Source         AccessSource
	PermissionMask int32
	Operations     []Operation
}

func (a Access) Allows(operation Operation) bool {
	for _, allowed := range a.Operations {
		if allowed == operation {
			return true
		}
	}
	return false
}

// ScopeMode distinguishes an empty scope from an explicit resource-ID scope.
type ScopeMode string

const (
	ScopeNone ScopeMode = "none"
	ScopeIDs  ScopeMode = "ids"
)

// Scope is a permission-filtered candidate set for list and retrieval queries.
type Scope struct {
	Mode        ScopeMode
	TenantID    string
	Kind        ResourceKind
	Parent      ResourceRef
	Operation   Operation
	ResourceIDs []string
}

// ScopeQuery describes the candidate resource set and optional execution entry.
type ScopeQuery struct {
	Kind      ResourceKind
	Parent    ResourceRef
	Operation Operation
	Entry     *ResourceRef
	EntryOp   Operation
}

// Source loads normalized permission facts. GetResources must return the
// requested resources with their tenant, creator, permission owner, active
// state, visibility, operation sets, and share recipients. ListResourceRefs
// returns all candidate IDs within the query boundary, including candidates
// that policy will reject; Checker authorizes them before returning a scope.
type Source interface {
	GetMembership(ctx context.Context, userID, tenantID string) (*Membership, error)
	GetResources(ctx context.Context, refs []ResourceRef) ([]Resource, error)
	ListResourceRefs(ctx context.Context, subject Subject, kind ResourceKind, parent ResourceRef) ([]ResourceRef, error)
}

type policy interface {
	CheckFeature(context.Context, Subject, Feature, Action) error
	CheckTenant(context.Context, Subject, string, TenantRequirement) error
	CheckResource(context.Context, Subject, ResourceRef, Operation) error
	CheckDependency(context.Context, Subject, ResourceRef, ResourceRef, Operation, Operation) error
	ResolveAccess(context.Context, Subject, ResourceRef) (Access, error)
	Scope(context.Context, Subject, ScopeQuery) (Scope, error)
	FilterResources(context.Context, Subject, []ResourceRef, Operation) ([]ResourceRef, error)
}

type queryScopePolicy interface {
	QueryScope(context.Context, Subject, ScopeQuery) (QueryScope, bool, error)
}

type accessResolverPolicy interface {
	ResolveAccesses(context.Context, Subject, []ResourceRef) (map[ResourceRef]Access, error)
}

type resourceOwnerGranter interface {
	GrantResourceOwner(context.Context, Subject, ResourceRef) error
}

// Checker exposes edition-independent permission methods.
type Checker struct {
	policy policy
}

// NewChecker creates a checker backed by normalized membership and resource facts.
func NewChecker(source Source) *Checker {
	if source == nil {
		return &Checker{}
	}
	return &Checker{policy: newPolicy(source)}
}

// GrantResourceOwner assigns the creator's owner grant for policies that use
// explicit resource permissions. Policies without grant rows keep their own
// ownership model.
func (c *Checker) GrantResourceOwner(ctx context.Context, subject Subject, resource ResourceRef) error {
	if c == nil || c.policy == nil {
		return ErrSourceUnavailable
	}
	granter, ok := c.policy.(resourceOwnerGranter)
	if !ok {
		return nil
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return err
	}
	return granter.GrantResourceOwner(ctx, subject, resource)
}

// CheckFeature checks feature access under the active edition's policy.
func (c *Checker) CheckFeature(ctx context.Context, subject Subject, feature Feature, action Action) error {
	if c == nil || c.policy == nil {
		return ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return err
	}
	return c.policy.CheckFeature(ctx, subject, feature, action)
}

// CheckTenant checks the user's membership and role in tenantID.
func (c *Checker) CheckTenant(ctx context.Context, subject Subject, tenantID string, requirement TenantRequirement) error {
	if c == nil || c.policy == nil {
		return ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return err
	}
	return c.policy.CheckTenant(ctx, subject, tenantID, requirement)
}

// CheckResource checks a direct operation on a resource.
func (c *Checker) CheckResource(ctx context.Context, subject Subject, resource ResourceRef, operation Operation) error {
	if c == nil || c.policy == nil {
		return ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return err
	}
	return c.policy.CheckResource(ctx, subject, resource, operation)
}

// CheckDependency checks entry access and the user's and entry owner's access to a dependency.
func (c *Checker) CheckDependency(ctx context.Context, subject Subject, entry, dependency ResourceRef, entryOperation, dependencyOperation Operation) error {
	if c == nil || c.policy == nil {
		return ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return err
	}
	return c.policy.CheckDependency(ctx, subject, entry, dependency, entryOperation, dependencyOperation)
}

// ResolveAccess resolves the effective operations and grant source for a resource.
func (c *Checker) ResolveAccess(ctx context.Context, subject Subject, resource ResourceRef) (Access, error) {
	if c == nil || c.policy == nil {
		return Access{}, ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return Access{}, err
	}
	return c.policy.ResolveAccess(ctx, subject, resource)
}

// Scope returns the explicitly authorized IDs for a resource query.
func (c *Checker) Scope(ctx context.Context, subject Subject, query ScopeQuery) (Scope, error) {
	if c == nil || c.policy == nil {
		return Scope{}, ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return Scope{}, err
	}
	return c.policy.Scope(ctx, subject, query)
}

// QueryScope returns a database-applied authorization predicate for a resource
// query. Policies may compile the authorization rules into SQL. When they do
// not, the checker falls back to the existing authorized-ID scope and applies
// those IDs as an IN predicate.
func (c *Checker) QueryScope(ctx context.Context, subject Subject, query ScopeQuery) (QueryScope, error) {
	if c == nil || c.policy == nil {
		return QueryScope{}, ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return QueryScope{}, err
	}
	if queryPolicy, ok := c.policy.(queryScopePolicy); ok {
		queryScope, supported, err := queryPolicy.QueryScope(ctx, subject, query)
		if err != nil {
			return QueryScope{}, err
		}
		if supported {
			return queryScope, nil
		}
	}

	scope, err := c.policy.Scope(ctx, subject, query)
	if err != nil {
		return QueryScope{}, err
	}
	return queryScopeForIDs(query.Kind, scope.ResourceIDs), nil
}

// FilterResources returns only candidate resources that allow operation.
func (c *Checker) FilterResources(ctx context.Context, subject Subject, resources []ResourceRef, operation Operation) ([]ResourceRef, error) {
	if c == nil || c.policy == nil {
		return nil, ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return nil, err
	}
	return c.policy.FilterResources(ctx, subject, resources, operation)
}

// ResolveAccesses resolves effective access for a batch of resources.
// Database-backed policies can load resource facts once for the whole batch.
func (c *Checker) ResolveAccesses(ctx context.Context, subject Subject, resources []ResourceRef) (map[ResourceRef]Access, error) {
	if c == nil || c.policy == nil {
		return nil, ErrSourceUnavailable
	}
	var err error
	if subject, err = resolveSubject(ctx, subject); err != nil {
		return nil, err
	}
	if batchPolicy, ok := c.policy.(accessResolverPolicy); ok {
		return batchPolicy.ResolveAccesses(ctx, subject, resources)
	}

	accesses := make(map[ResourceRef]Access, len(resources))
	for _, resource := range uniqueRefs(resources) {
		access, err := c.policy.ResolveAccess(ctx, subject, resource)
		if err != nil {
			if errorsIsPermissionDenial(err) || errors.Is(err, ErrResourceNotFound) {
				continue
			}
			return nil, err
		}
		accesses[resource] = access
	}
	return accesses, nil
}

// FilterAccesses resolves permissions for a batch of resources and returns the
// effective access for each resource that allows operation.
func (c *Checker) FilterAccesses(ctx context.Context, subject Subject, resources []ResourceRef, operation Operation) (map[ResourceRef]Access, error) {
	if c == nil || c.policy == nil {
		return nil, ErrSourceUnavailable
	}
	if !validOperation(operation) {
		return nil, ErrInvalidPermission
	}
	accesses, err := c.ResolveAccesses(ctx, subject, resources)
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

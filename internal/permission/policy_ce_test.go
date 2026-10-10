//go:build !enterprise

package permission

import (
	"context"
	"errors"
	"testing"
)

type permissionTestSource struct {
	memberships map[string]*Membership
	resources   map[ResourceRef]Resource
	candidates  []ResourceRef
}

func (s *permissionTestSource) GetMembership(_ context.Context, userID, tenantID string) (*Membership, error) {
	return s.memberships[userID+"\x00"+tenantID], nil
}

func (s *permissionTestSource) GetResources(_ context.Context, refs []ResourceRef) ([]Resource, error) {
	resources := make([]Resource, 0, len(refs))
	for _, ref := range refs {
		if resource, ok := s.resources[ref]; ok {
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

func (s *permissionTestSource) ListResourceRefs(_ context.Context, _ Subject, kind ResourceKind, parent ResourceRef) ([]ResourceRef, error) {
	refs := make([]ResourceRef, 0, len(s.candidates))
	for _, ref := range s.candidates {
		if ref.Kind == kind {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func TestCommunityPolicyResolvesResourceTenant(t *testing.T) {
	resourceRef := ResourceRef{Kind: "dataset", ID: "dataset-1"}
	source := &permissionTestSource{
		memberships: map[string]*Membership{
			"user-1\x00tenant-resource": {
				UserID: "user-1", TenantID: "tenant-resource", Role: RoleNormal, Active: true,
			},
		},
		resources: map[ResourceRef]Resource{
			resourceRef: {
				Ref: resourceRef, TenantID: "tenant-resource", OwnerUserID: "tenant-owner",
				Visibility: VisibilityTenant, OwnerOperations: []Operation{OperationRead}, Active: true,
			},
		},
	}

	access, err := NewChecker(source).ResolveAccess(context.Background(), Subject{UserID: "user-1"}, resourceRef)
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.TenantID != "tenant-resource" {
		t.Fatalf("ResolveAccess() tenant = %q, want resource tenant", access.TenantID)
	}
	if access.Source != AccessSourceTenant || !access.Allows(OperationRead) {
		t.Fatalf("ResolveAccess() = %#v, want tenant read access", access)
	}
}

func TestCommunityPolicyUsesResourceTenantInsteadOfSubjectTenant(t *testing.T) {
	resourceRef := ResourceRef{Kind: "dataset", ID: "dataset-1"}
	source := &permissionTestSource{
		memberships: map[string]*Membership{
			"user-1\x00tenant-resource": {
				UserID: "user-1", TenantID: "tenant-resource", Role: RoleNormal, Active: true,
			},
		},
		resources: map[ResourceRef]Resource{
			resourceRef: {
				Ref: resourceRef, TenantID: "tenant-resource", OwnerUserID: "tenant-owner",
				Visibility: VisibilityTenant, OwnerOperations: []Operation{OperationRead}, Active: true,
			},
		},
	}

	access, err := NewChecker(source).ResolveAccess(context.Background(), Subject{UserID: "user-1", TenantID: "another-tenant"}, resourceRef)
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.TenantID != "tenant-resource" {
		t.Fatalf("ResolveAccess() tenant = %q, want resource tenant", access.TenantID)
	}

	_, err = NewChecker(source).ResolveAccess(context.Background(), Subject{UserID: "unrelated-user", TenantID: "tenant-resource"}, resourceRef)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ResolveAccess() error = %v, want %v for a non-member", err, ErrPermissionDenied)
	}
}

func TestCommunityPolicySharedResourceNeedsNoSubjectTenant(t *testing.T) {
	resourceRef := ResourceRef{Kind: "agent", ID: "agent-1"}
	source := &permissionTestSource{
		memberships: map[string]*Membership{
			"user-1\x00shared-tenant": {
				UserID: "user-1", TenantID: "shared-tenant", Role: RoleNormal, Active: true,
			},
		},
		resources: map[ResourceRef]Resource{
			resourceRef: {
				Ref: resourceRef, TenantID: "owner-tenant", OwnerUserID: "owner-user",
				Visibility: VisibilityShared, SharedWithTenantIDs: []string{"shared-tenant"},
				OwnerOperations: []Operation{OperationRun}, Active: true,
			},
		},
	}

	access, err := NewChecker(source).ResolveAccess(context.Background(), Subject{UserID: "user-1"}, resourceRef)
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Source != AccessSourceShared || !access.Allows(OperationRun) {
		t.Fatalf("ResolveAccess() = %#v, want shared run access", access)
	}
	if err := NewChecker(source).CheckResource(context.Background(), Subject{UserID: "user-1"}, resourceRef, OperationShare); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CheckResource(share) error = %v, want %v for a shared resource", err, ErrPermissionDenied)
	}
}

func TestCommunityPolicyChecksDependencyForBothUserAndEntryOwner(t *testing.T) {
	entryRef := ResourceRef{Kind: "agent", ID: "agent-1"}
	dependencyRef := ResourceRef{Kind: "dataset", ID: "dataset-1"}
	source := &permissionTestSource{
		resources: map[ResourceRef]Resource{
			entryRef: {
				Ref: entryRef, TenantID: "entry-tenant", OwnerUserID: "entry-owner",
				Visibility: VisibilityShared, SharedWithUserIDs: []string{"caller"},
				OwnerOperations: []Operation{OperationRun}, Active: true,
			},
			dependencyRef: {
				Ref: dependencyRef, TenantID: "dataset-tenant", OwnerUserID: "dataset-owner",
				Visibility: VisibilityShared, SharedWithUserIDs: []string{"caller"},
				OwnerOperations: []Operation{OperationRead}, Active: true,
			},
		},
	}

	err := NewChecker(source).CheckDependency(
		context.Background(), Subject{UserID: "caller"}, entryRef, dependencyRef, OperationRun, OperationRead,
	)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CheckDependency() error = %v, want %v", err, ErrPermissionDenied)
	}

	dependency := source.resources[dependencyRef]
	dependency.SharedWithUserIDs = []string{"caller", "entry-owner"}
	source.resources[dependencyRef] = dependency
	err = NewChecker(source).CheckDependency(
		context.Background(), Subject{UserID: "caller"}, entryRef, dependencyRef, OperationRun, OperationRead,
	)
	if err != nil {
		t.Fatalf("CheckDependency() error = %v, want nil when both caller and entry owner can read dependency", err)
	}
}

func TestCommunityPolicyFeatureDoesNotRequireTenantContext(t *testing.T) {
	checker := NewChecker(&permissionTestSource{})
	if err := checker.CheckFeature(context.Background(), Subject{UserID: "user-1"}, "dataset", ActionRead); err != nil {
		t.Fatalf("CheckFeature() error = %v", err)
	}
}

func TestCommunityPolicyScopeUsesParentResourceTenant(t *testing.T) {
	parentRef := ResourceRef{Kind: "dataset", ID: "dataset-1"}
	childRef := ResourceRef{Kind: "document", ID: "document-1"}
	source := &permissionTestSource{
		memberships: map[string]*Membership{
			"user-1\x00dataset-tenant": {
				UserID: "user-1", TenantID: "dataset-tenant", Role: RoleNormal, Active: true,
			},
		},
		resources: map[ResourceRef]Resource{
			parentRef: {
				Ref: parentRef, TenantID: "dataset-tenant", OwnerUserID: "dataset-owner",
				Visibility: VisibilityTenant, OwnerOperations: []Operation{OperationRead}, Active: true,
			},
			childRef: {
				Ref: childRef, TenantID: "dataset-tenant", OwnerUserID: "dataset-owner",
				Visibility: VisibilityTenant, OwnerOperations: []Operation{OperationRead}, Active: true,
			},
		},
		candidates: []ResourceRef{childRef},
	}

	scope, err := NewChecker(source).Scope(context.Background(), Subject{UserID: "user-1"}, ScopeQuery{
		Kind: "document", Parent: parentRef, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope() error = %v", err)
	}
	if scope.TenantID != "dataset-tenant" {
		t.Fatalf("Scope() tenant = %q, want parent tenant", scope.TenantID)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != childRef.ID {
		t.Fatalf("Scope() = %#v, want the accessible child ID", scope)
	}
}

func TestCommunityPolicyRequiresUserIdentity(t *testing.T) {
	resourceRef := ResourceRef{Kind: "dataset", ID: "dataset-1"}
	_, err := NewChecker(&permissionTestSource{}).ResolveAccess(context.Background(), Subject{}, resourceRef)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("ResolveAccess() error = %v, want %v", err, ErrUnauthenticated)
	}
}

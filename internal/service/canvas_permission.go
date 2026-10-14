package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// CheckCanvasPermission checks a canvas operation through the active permission policy.
func CheckCanvasPermission(ctx context.Context, subject permission.Subject, canvasID string, operation permission.Operation) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(
		ctx,
		subject,
		permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID},
		operation,
	)
}

// AccessibleCanvasIDs returns the canvas IDs visible for an operation.
func AccessibleCanvasIDs(ctx context.Context, subject permission.Subject, operation permission.Operation) ([]string, error) {
	scope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, subject, permission.ScopeQuery{
		Kind:      permission.ResourceKindCanvas,
		Operation: operation,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode == permission.ScopeNone {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

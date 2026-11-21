package runtime

import (
	"context"
	"fmt"
)

type datasetTenantsKey struct{}

// DatasetTenantScope pairs dataset IDs with the tenant that stores their chunks.
type DatasetTenantScope struct {
	TenantID   string
	DatasetIDs []string
}

// WithDatasetTenants attaches the server-resolved owners of an authorized dataset scope.
func WithDatasetTenants(ctx context.Context, tenants map[string]string) context.Context {
	return context.WithValue(ctx, datasetTenantsKey{}, tenants)
}

// GroupDatasetTenantScopes groups a requested subset by its storage tenant.
// Callers without a bound dataset scope retain their original tenant routing.
func GroupDatasetTenantScopes(ctx context.Context, tenantID string, datasetIDs []string) ([]DatasetTenantScope, error) {
	tenants, bound := ctx.Value(datasetTenantsKey{}).(map[string]string)
	if !bound {
		return []DatasetTenantScope{{TenantID: tenantID, DatasetIDs: datasetIDs}}, nil
	}
	scopes := make([]DatasetTenantScope, 0)
	positions := make(map[string]int)
	for _, id := range datasetIDs {
		owner := tenants[id]
		if owner == "" {
			return nil, fmt.Errorf("dataset %q is outside the retrieval scope", id)
		}
		position, ok := positions[owner]
		if !ok {
			position = len(scopes)
			positions[owner] = position
			scopes = append(scopes, DatasetTenantScope{TenantID: owner})
		}
		scopes[position].DatasetIDs = append(scopes[position].DatasetIDs, id)
	}
	return scopes, nil
}

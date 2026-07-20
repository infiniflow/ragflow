package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// HasKBTeamPermission mirrors Python check_kb_team_permission:
// direct owner access is always allowed; otherwise the KB must be team-shared
// and the caller must be a joined normal member of the owner tenant.
func HasKBTeamPermission(kb *entity.Knowledgebase, userID string, tenantDAO *dao.TenantDAO) bool {
	if kb == nil {
		return false
	}
	if kb.TenantID == userID {
		return true
	}
	if kb.Permission != string(entity.TenantPermissionTeam) {
		return false
	}
	joinedTenants, err := tenantDAO.GetJoinedTenantsByUserID(userID)
	if err != nil {
		return false
	}
	for _, tenant := range joinedTenants {
		if tenant.TenantID == kb.TenantID {
			return true
		}
	}
	return false
}

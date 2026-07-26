package dataset

import (
	"errors"
	"strings"

	"ragflow/internal/entity"
)

// Accessible checks if a user has access to a dataset.
func (d *DatasetService) Accessible(kbID, userID string) bool {
	return d.kbDAO.Accessible(kbID, userID)
}

// GetByID retrieves a knowledge base by ID.
func (d *DatasetService) GetByID(kbID string) (*entity.Knowledgebase, error) {
	return d.kbDAO.GetByID(kbID)
}

// GetKnowledgebaseByID resolves a dataset entity without applying permission
// checks. Upload needs the same existence-then-auth ordering as Python.
func (d *DatasetService) GetKnowledgebaseByID(datasetID string) (*entity.Knowledgebase, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, errors.New("Lack of \"Dataset ID\"")
	}
	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, err
	}
	return d.kbDAO.GetByID(normalizedID)
}

// CheckKBTeamPermission checks if a user has team-level permission for the KB.
func (d *DatasetService) CheckKBTeamPermission(kb *entity.Knowledgebase, userID string) bool {
	if kb == nil {
		return false
	}
	if kb.TenantID == userID {
		return true
	}
	if kb.Permission != string(entity.TenantPermissionTeam) {
		return false
	}
	joinedTenants, err := d.tenantDAO.GetJoinedTenantsByUserID(userID)
	if err != nil {
		return false
	}
	for _, jt := range joinedTenants {
		if jt != nil && jt.TenantID == kb.TenantID {
			return true
		}
	}
	return false
}

// GetFieldMap returns the field map for the given knowledge base IDs.
func (d *DatasetService) GetFieldMap(ids []string) (map[string]interface{}, error) {
	return d.kbDAO.GetFieldMap(ids)
}

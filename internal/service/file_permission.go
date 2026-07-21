package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// CheckFileTeamPermission reports whether userID may access the given file: either
// the file's owning tenant matches userID, or userID is a team member of any
// dataset the file is linked to. It mirrors Python's check_file_team_permission
// and is shared by FileService and File2DocumentService, which previously each
// carried an identical copy of this logic.
func CheckFileTeamPermission(fileDAO *dao.FileDAO, file *entity.File, userID string) bool {
	if file.TenantID == userID {
		return true
	}

	datasetIDs, err := fileDAO.GetDatasetIDByFileID(file.ID)
	if err != nil || len(datasetIDs) == 0 {
		return false
	}

	kbDAO := dao.NewKnowledgebaseDAO()
	for _, datasetID := range datasetIDs {
		kb, err := kbDAO.GetByID(datasetID)
		if err != nil || kb == nil {
			continue
		}
		if HasKBTeamPermission(kb, userID, dao.NewTenantDAO()) {
			return true
		}
	}
	return false
}

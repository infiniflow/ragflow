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

package dao

import (
	"context"
	"errors"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FileDAO file data access object
type FileDAO struct{}

// NewFileDAO create file DAO
func NewFileDAO() *FileDAO {
	return &FileDAO{}
}

// GetByID gets file by ID
func (dao *FileDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.File, error) {
	var file entity.File
	err := db.WithContext(ctx).Where("id = ?", id).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByIDAndTenant gets a file by ID scoped to the given tenant. Callers that
// resolve a user-supplied file ID (e.g. parser_config.tags.tag_file_id, which
// the dataset update API accepts from the client) MUST use this instead of
// GetByID: an unscoped ID lookup crosses tenant boundaries and is an IDOR
// (CWE-639). An empty id or tenantID fails closed with gorm.ErrRecordNotFound
// so callers see the same "not found" as a missing row.
func (dao *FileDAO) GetByIDAndTenant(ctx context.Context, db *gorm.DB, id, tenantID string) (*entity.File, error) {
	if id == "" || tenantID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var file entity.File
	err := db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByPfID gets files by parent folder ID with pagination and filtering.
// When keywords is empty, only direct children of pfID are listed; when
// keywords is non-empty, the search covers the whole subtree under pfID so
// files and folders nested in sub-folders can be found too.
func (dao *FileDAO) GetByPfID(ctx context.Context, db *gorm.DB, tenantID, pfID string, page, pageSize int, terms []OrderTerm, keywords string, excludeSkills bool, visibilityScopes ...func(*gorm.DB) *gorm.DB) ([]*entity.File, int64, error) {
	var files []*entity.File
	var total int64
	query, err := dao.fileListQuery(ctx, db, tenantID, pfID, keywords, excludeSkills)
	if err != nil {
		return nil, 0, err
	}
	if len(visibilityScopes) > 0 {
		if visibilityScopes[0] == nil {
			query = query.Where("1 = 0")
		} else {
			query = visibilityScopes[0](query)
		}
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering. Route orderBy through fileOrderClause so a user-supplied
	// query param can never reach Order() verbatim: the helper validates
	// against fileOrderableColumns (a closed allowlist) and falls back to
	// "create_time" on a miss.
	// codeql[go/sql-injection] False positive: fileOrderClause
	query = query.Order(fileOrderClause(terms))

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&files).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&files).Error; err != nil {
			return nil, 0, err
		}
	}

	return files, total, nil
}

func (dao *FileDAO) fileListQuery(ctx context.Context, db *gorm.DB, tenantID, pfID, keywords string, excludeSkills bool) (*gorm.DB, error) {
	query := db.WithContext(ctx).Model(&entity.File{}).
		Where("tenant_id = ? AND id != ?", tenantID, pfID)
	if keywords != "" {
		descendantIDs, err := dao.GetSubtreeIDs(ctx, db, tenantID, pfID)
		if err != nil {
			return nil, err
		}
		query = query.Where("parent_id IN ?", descendantIDs).
			Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	} else {
		query = query.Where("parent_id = ?", pfID)
	}
	if excludeSkills {
		query = query.Where("NOT (parent_id = ? AND name = ?)", pfID, SkillsFolderName)
	}
	return query, nil
}

// ListDescendants loads every direct and nested entry under a folder. It walks
// folder levels in batches so the caller can authorize a whole tree without a
// database round trip for every node. Parent IDs are chunked to stay below
// database parameter limits.
func (dao *FileDAO) ListDescendants(ctx context.Context, db *gorm.DB, folderID string) ([]entity.File, error) {
	const parentBatchSize = 500
	if folderID == "" {
		return nil, nil
	}
	seen := map[string]struct{}{folderID: {}}
	frontier := []string{folderID}
	descendants := make([]entity.File, 0)
	for len(frontier) > 0 {
		next := make([]string, 0)
		for start := 0; start < len(frontier); start += parentBatchSize {
			end := start + parentBatchSize
			if end > len(frontier) {
				end = len(frontier)
			}
			var children []entity.File
			if err := db.WithContext(ctx).Model(&entity.File{}).
				Select("id", "parent_id", "type").
				Where("parent_id IN ?", frontier[start:end]).
				Find(&children).Error; err != nil {
				return nil, err
			}
			for i := range children {
				child := children[i]
				if _, exists := seen[child.ID]; exists {
					continue
				}
				seen[child.ID] = struct{}{}
				descendants = append(descendants, child)
				if child.Type == "folder" {
					next = append(next, child.ID)
				}
			}
		}
		frontier = next
	}
	return descendants, nil
}

// GetSubtreeIDs returns pfID itself plus the IDs of all entries nested under
// it (folders and files), used to scope recursive keyword searches.
func (dao *FileDAO) GetSubtreeIDs(ctx context.Context, db *gorm.DB, tenantID, pfID string) ([]string, error) {
	var rows []struct {
		ID       string
		ParentID string
	}
	if err := db.WithContext(ctx).Model(&entity.File{}).
		Select("id", "parent_id").
		Where("tenant_id = ?", tenantID).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	children := make(map[string][]string, len(rows))
	for _, row := range rows {
		children[row.ParentID] = append(children[row.ParentID], row.ID)
	}

	ids := []string{pfID}
	inTree := map[string]struct{}{pfID: {}}
	queue := []string{pfID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range children[cur] {
			if _, ok := inTree[child]; ok {
				continue
			}
			inTree[child] = struct{}{}
			ids = append(ids, child)
			queue = append(queue, child)
		}
	}
	return ids, nil
}

// GetRootFolder gets or creates root folder for tenant
func (dao *FileDAO) GetRootFolder(ctx context.Context, db *gorm.DB, tenantID string) (*entity.File, error) {
	var file entity.File
	err := db.WithContext(ctx).Where("tenant_id = ? AND parent_id = id", tenantID).First(&file).Error
	if err == nil {
		return &file, nil
	}

	// Create root folder if not exists
	fileID := common.GenerateToken()
	file = entity.File{
		ID:        fileID,
		ParentID:  fileID,
		UserID:    tenantID,
		TenantID:  tenantID,
		CreatedBy: tenantID,
		Name:      "/",
		Type:      "folder",
		Size:      0,
	}
	file.SourceType = ""

	if err = db.WithContext(ctx).Create(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

// GetParentFolder gets parent folder of a file
func (dao *FileDAO) GetParentFolder(ctx context.Context, db *gorm.DB, fileID string) (*entity.File, error) {
	var file entity.File
	err := db.WithContext(ctx).Where("id = ?", fileID).First(&file).Error
	if err != nil {
		return nil, err
	}

	var parentFile entity.File
	err = db.WithContext(ctx).Where("id = ?", file.ParentID).First(&parentFile).Error
	if err != nil {
		return nil, err
	}
	return &parentFile, nil
}

// ListByParentID lists all files by parent ID (including subfolders)
func (dao *FileDAO) ListByParentID(ctx context.Context, db *gorm.DB, parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := db.WithContext(ctx).Where("parent_id = ? AND id != ?", parentID, parentID).Find(&files).Error
	return files, err
}

// GetFolderSize calculates folder size recursively
func (dao *FileDAO) GetFolderSize(ctx context.Context, db *gorm.DB, folderID string) (int64, error) {
	var size int64

	var dfs func(parentID string) error
	dfs = func(parentID string) error {
		var files []*entity.File
		if err := db.WithContext(ctx).Select("id", "size", "type").
			Where("parent_id = ? AND id != ?", parentID, parentID).
			Find(&files).Error; err != nil {
			return err
		}

		for _, f := range files {
			size += f.Size
			if f.Type == "folder" {
				if err := dfs(f.ID); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := dfs(folderID); err != nil {
		return 0, err
	}
	return size, nil
}

// HasChildFolder checks if folder has child folders
func (dao *FileDAO) HasChildFolder(ctx context.Context, db *gorm.DB, folderID string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.File{}).
		Where("parent_id = ? AND id != ? AND type = ?", folderID, folderID, "folder").
		Count(&count).Error
	return count > 0, err
}

// GetAllParentFolders gets all parent folders in path (from current to root)
func (dao *FileDAO) GetAllParentFolders(ctx context.Context, db *gorm.DB, startID string) ([]*entity.File, error) {
	var parentFolders []*entity.File
	currentID := startID

	for currentID != "" {
		var file entity.File
		err := db.WithContext(ctx).Where("id = ?", currentID).First(&file).Error
		if err != nil {
			return nil, err
		}

		parentFolders = append(parentFolders, &file)

		// Stop if we've reached the root folder (parent_id == id)
		if file.ParentID == file.ID {
			break
		}
		currentID = file.ParentID
	}

	return parentFolders, nil
}

// Create creates a new file
func (dao *FileDAO) Create(ctx context.Context, db *gorm.DB, file *entity.File) error {
	if file.UserID == "" {
		file.UserID = file.CreatedBy
	}
	return db.WithContext(ctx).Create(file).Error
}

// UpdateByID updates a file by ID
func (dao *FileDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.File{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteByIDs deletes files by IDs (hard delete)
func (dao *FileDAO) DeleteByIDs(ctx context.Context, db *gorm.DB, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&entity.File{})
	return result.RowsAffected, result.Error
}

// GetAllIDsByTenantID gets all file IDs by tenant ID
func (dao *FileDAO) GetAllIDsByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]string, error) {
	var ids []string
	err := db.WithContext(ctx).Model(&entity.File{}).Where("tenant_id = ?", tenantID).Pluck("id", &ids).Error
	return ids, err
}

// GetByIDs gets files by multiple IDs
func (dao *FileDAO) GetByIDs(ctx context.Context, db *gorm.DB, ids []string) ([]*entity.File, error) {
	var files []*entity.File
	if len(ids) == 0 {
		return files, nil
	}
	err := db.WithContext(ctx).Where("id IN ?", ids).Find(&files).Error
	return files, err
}

// ListAllFilesByParentID lists all files by parent folder ID
func (dao *FileDAO) ListAllFilesByParentID(ctx context.Context, db *gorm.DB, parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := db.WithContext(ctx).Where("parent_id = ? AND id != ?", parentID, parentID).Find(&files).Error
	return files, err
}

// ListNonFolderByParentID lists non-folder files directly under a parent folder.
func (dao *FileDAO) ListNonFolderByParentID(ctx context.Context, db *gorm.DB, parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := db.WithContext(ctx).Where("parent_id = ? AND id != ? AND type != ?", parentID, parentID, "folder").Find(&files).Error
	return files, err
}

// ListFolderByParentID lists sub-folders directly under a parent folder.
func (dao *FileDAO) ListFolderByParentID(ctx context.Context, db *gorm.DB, parentID string) ([]*entity.File, error) {
	var files []*entity.File
	err := db.WithContext(ctx).Where("parent_id = ? AND type = ?", parentID, "folder").Find(&files).Error
	return files, err
}

// GetByParentIDAndName gets file by parent folder ID and name
func (dao *FileDAO) GetByParentIDAndName(ctx context.Context, db *gorm.DB, parentID, name string) (*entity.File, error) {
	var file entity.File
	err := db.WithContext(ctx).Where("parent_id = ? AND name = ?", parentID, name).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetIDListByID recursively gets list of file IDs by traversing folder structure
func (dao *FileDAO) GetIDListByID(ctx context.Context, db *gorm.DB, id string, names []string, count int, res []string) ([]string, error) {
	if count < len(names) {
		file, err := dao.GetByParentIDAndName(ctx, db, id, names[count])
		if err != nil {
			return res, nil
		}
		res = append(res, file.ID)
		return dao.GetIDListByID(ctx, db, file.ID, names, count+1, res)
	}
	return res, nil
}

// CreateFolder creates a folder in the database
func (dao *FileDAO) CreateFolder(ctx context.Context, db *gorm.DB, parentID, tenantID, createdBy, name, fileType string) (*entity.File, error) {
	file := &entity.File{
		ID:         common.GenerateToken(),
		ParentID:   parentID,
		UserID:     createdBy,
		TenantID:   tenantID,
		CreatedBy:  createdBy,
		Name:       name,
		Type:       fileType,
		Size:       0,
		SourceType: "",
	}
	if err := db.WithContext(ctx).Create(file).Error; err != nil {
		return nil, err
	}
	return file, nil
}

// Insert inserts a new file record
func (dao *FileDAO) Insert(ctx context.Context, db *gorm.DB, file *entity.File) error {
	if file.UserID == "" {
		file.UserID = file.CreatedBy
	}
	return db.WithContext(ctx).Create(file).Error
}

// IsParentFolderExist checks if parent folder exists
func (dao *FileDAO) IsParentFolderExist(ctx context.Context, db *gorm.DB, parentID string) bool {
	var count int64
	err := db.WithContext(ctx).Model(&entity.File{}).Where("id = ?", parentID).Count(&count).Error
	if err != nil || count == 0 {
		return false
	}
	return true
}

// Query retrieves files by conditions
func (dao *FileDAO) Query(ctx context.Context, db *gorm.DB, name string, parentID string, tenantID string) ([]*entity.File, error) {
	var files []*entity.File
	query := db.WithContext(ctx).Model(&entity.File{})
	if name != "" {
		query = query.Where("name = ?", name)
	}
	if parentID != "" {
		query = query.Where("parent_id = ?", parentID)
	}
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// NameExists reports whether a file or folder with the given name already
// exists under parentID for the tenant, comparing names case-insensitively.
// excludeID, when non-empty, omits that entry from the check so moving or
// renaming an entry does not collide with itself.
func (dao *FileDAO) NameExists(ctx context.Context, db *gorm.DB, name, parentID, tenantID, excludeID string) (bool, error) {
	q := db.WithContext(ctx).Model(&entity.File{}).
		Where("LOWER(name) = LOWER(?) AND parent_id = ? AND tenant_id = ?", name, parentID, tenantID)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// Delete deletes a file by ID (hard delete)
func (dao *FileDAO) Delete(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&entity.File{}).Error
}

// GetDatasetIDByFileID gets dataset ID by file ID
func (dao *FileDAO) GetDatasetIDByFileID(ctx context.Context, db *gorm.DB, fileID string) ([]string, error) {
	var datasetIDs []string
	rows, err := db.WithContext(ctx).Model(&entity.File{}).
		Select("knowledgebase.id").
		Joins("JOIN file2document ON file2document.file_id = ?", fileID).
		Joins("JOIN document ON document.id = file2document.document_id").
		Joins("JOIN knowledgebase ON knowledgebase.id = document.kb_id").
		Where("file.id = ?", fileID).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var kbID string
		if err := rows.Scan(&kbID); err != nil {
			continue
		}
		datasetIDs = append(datasetIDs, kbID)
	}

	return datasetIDs, nil
}

// DatasetFolderName is the folder name for dataset
const DatasetFolderName = ".knowledgebase"

// SkillsFolderName is the folder name for skills
const SkillsFolderName = "skills"

// InitDatasetDocs initializes dataset folders and document mappings.
func (dao *FileDAO) InitDatasetDocs(ctx context.Context, db *gorm.DB, rootID, tenantID string, file2DocumentDAO *File2DocumentDAO) error {
	var existing []entity.File
	if err := db.WithContext(ctx).Where("name = ? AND parent_id = ? AND tenant_id = ?", DatasetFolderName, rootID, tenantID).Limit(2).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) > 1 || (len(existing) == 1 && existing[0].Type != "folder") {
		common.Warn("Retaining ambiguous dataset root", zap.String("tenant_id", tenantID))
		return nil
	}
	if len(existing) == 1 {
		return nil
	}
	if _, err := datasetRoot(ctx, db, tenantID, true); err != nil {
		return err
	}
	var datasets []entity.Knowledgebase
	if err := db.WithContext(ctx).Select("id", "name", "tenant_id").Where("tenant_id = ?", tenantID).Find(&datasets).Error; err != nil {
		return err
	}
	for _, kb := range datasets {
		folder, err := dao.EnsureDatasetFolder(ctx, db, &kb)
		if err != nil {
			if errors.Is(err, ErrDatasetFolderConflict) {
				common.Warn("Skipping ambiguous dataset folder", zap.String("kb_id", kb.ID))
				continue
			}
			return err
		}
		var documents []entity.Document
		if err := db.WithContext(ctx).Where("kb_id = ?", kb.ID).Find(&documents).Error; err != nil {
			return err
		}
		for _, doc := range documents {
			if err := dao.addFileFromKB(ctx, db, &doc, folder.ID, tenantID, file2DocumentDAO); err != nil {
				return err
			}
		}
	}
	return nil
}

// addFileFromKB adds a file record from knowledgebase document
func (dao *FileDAO) addFileFromKB(ctx context.Context, db *gorm.DB, doc *entity.Document, datasetFolderID, tenantID string, file2DocumentDAO *File2DocumentDAO) error {
	var f2dCount int64
	err := db.WithContext(ctx).Model(&entity.File2Document{}).
		Where("document_id = ?", doc.ID).
		Count(&f2dCount).Error
	if err != nil {
		return err
	}

	if f2dCount > 0 {
		return nil
	}

	docName := ""
	if doc.Name != nil {
		docName = *doc.Name
	}

	docLocation := ""
	if doc.Location != nil {
		docLocation = *doc.Location
	}

	fileID := common.GenerateToken()
	file := &entity.File{
		ID:         fileID,
		ParentID:   datasetFolderID,
		UserID:     tenantID,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       docName,
		Type:       doc.Type,
		Size:       doc.Size,
		Location:   &docLocation,
		SourceType: "knowledgebase",
	}

	if err = db.WithContext(ctx).Create(file).Error; err != nil {
		return err
	}

	f2dID := common.GenerateToken()
	f2d := &entity.File2Document{
		ID:         f2dID,
		FileID:     &fileID,
		DocumentID: &doc.ID,
	}

	if err = db.WithContext(ctx).Create(f2d).Error; err != nil {
		return err
	}

	return nil
}

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

package file

import (
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

var (
	// assertURLSafe and pinnedHTTPClient are aliased from utility so tests
	// can override them for mock injection.
	assertURLSafe    = utility.AssertURLSafe
	pinnedHTTPClient = utility.PinnedHTTPClient
)

// DocRemover is the narrow interface FileService needs from the document domain.
type DocRemover interface {
	RemoveDocumentKeepFile(docID string) error
}

// CheckFilePermFunc is the function signature for file-team permission checks,
// injected by the parent adapter so the file subpackage does not need to import
// the parent service package.
type CheckFilePermFunc func(fileDAO *dao.FileDAO, file *entity.File, userID string) bool

// FileService file service
type FileService struct {
	fileDAO          *dao.FileDAO
	file2DocumentDAO *dao.File2DocumentDAO
	documentService  DocRemover
	checkFilePerm    CheckFilePermFunc
}

// NewFileService create file service. checkFilePerm is always required;
// dr may be nil when the caller only uses read/parse methods and never
// calls DeleteFiles.
func NewFileService(checkFilePerm CheckFilePermFunc, dr DocRemover) *FileService {
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  dr,
		checkFilePerm:    checkFilePerm,
	}
}

// FileInfo file info with additional fields
type FileInfo struct {
	*entity.File
	Size           int64                    `json:"size"`
	KbsInfo        []map[string]interface{} `json:"kbs_info"`
	HasChildFolder bool                     `json:"has_child_folder,omitempty"`
}

// ListFilesResponse list files response
type ListFilesResponse struct {
	Total        int64                    `json:"total"`
	Files        []map[string]interface{} `json:"files"`
	ParentFolder map[string]interface{}   `json:"parent_folder"`
}

// DatasetFolderName is the folder name for dataset
const DatasetFolderName = ".knowledgebase"

// SkillsFolderName is the folder name for skills
const SkillsFolderName = "skills"

// FileSourceDataset represents dataset as file source
const FileSourceDataset = "knowledgebase"

const (
	FileTypeFolder  = "folder"
	FileTypeVirtual = "virtual"
)

// MoveFileReq represents the request body for move files operation
type MoveFileReq struct {
	SrcFileIDs []string `json:"src_file_ids" binding:"required,min=1"`
	DestFileID string   `json:"dest_file_id"`
	NewName    string   `json:"new_name"`
}

// StorageAddress represents bucket and object name for storage
type StorageAddress struct {
	Bucket string
	Name   string
}

// maxRemoteFileSize bounds the body of a ?url= upload (100 MB).
const maxRemoteFileSize = 100 << 20

// reservedDeviceNames are Windows reserved filenames that must never be used.
var reservedDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

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

package entity

// FileCommit represents a snapshot commit for a workspace folder (like git commit).
type FileCommit struct {
	ID        string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	FolderID  string  `gorm:"column:folder_id;size:32;not null;index" json:"folder_id"`
	ParentID  *string `gorm:"column:parent_id;size:32;index" json:"parent_id,omitempty"`
	Message   string  `gorm:"column:message;size:512;not null;default:''" json:"message"`
	AuthorID  string  `gorm:"column:author_id;size:32;not null;index" json:"author_id"`
	FileCount int     `gorm:"column:file_count;default:0" json:"file_count"`
	TreeState *string `gorm:"column:tree_state;type:longtext" json:"tree_state,omitempty"`
	BaseModel
}

// TableName returns the table name for FileCommit model.
func (FileCommit) TableName() string {
	return "file_commit"
}

// FileCommitItem represents a single file change within a commit.
type FileCommitItem struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	CommitID    string  `gorm:"column:commit_id;size:32;not null;uniqueIndex:idx_commit_file" json:"commit_id"`
	FileID      string  `gorm:"column:file_id;size:32;not null;uniqueIndex:idx_commit_file" json:"file_id"`
	Operation   string  `gorm:"column:operation;size:16;not null;index" json:"operation"`
	OldHash     *string `gorm:"column:old_hash;size:64;index" json:"old_hash,omitempty"`
	NewHash     *string `gorm:"column:new_hash;size:64;index" json:"new_hash,omitempty"`
	OldLocation *string `gorm:"column:old_location;size:255" json:"old_location,omitempty"`
	NewLocation *string `gorm:"column:new_location;size:255" json:"new_location,omitempty"`
	OldName     *string `gorm:"column:old_name;size:255" json:"old_name,omitempty"`
	NewName     *string `gorm:"column:new_name;size:255" json:"new_name,omitempty"`
	BaseModel
}

// TableName returns the table name for FileCommitItem model.
func (FileCommitItem) TableName() string {
	return "file_commit_item"
}

// TreeNode represents a file node in the commit tree state snapshot.
type TreeNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" or "folder"
	Hash     string      `json:"hash,omitempty"`
	Location string      `json:"location,omitempty"`
	Size     int64       `json:"size,omitempty"`
	Status   string      `json:"status"` // "1" = active, "0" = deleted
	Children []*TreeNode `json:"children,omitempty"`
}

// FileChange represents a file change in a commit request.
type FileChange struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	Operation string `json:"operation"` // "add", "modify", "delete", "rename"
	Content   string `json:"content,omitempty"`
	OldName   string `json:"old_name,omitempty"`
	NewName   string `json:"new_name,omitempty"`
}

// CommitResponse is the API response for a commit.
type CommitResponse struct {
	ID         string  `json:"id"`
	FolderID   string  `json:"folder_id"`
	ParentID   *string `json:"parent_id,omitempty"`
	Message    string  `json:"message"`
	AuthorID   string  `json:"author_id"`
	FileCount  int     `json:"file_count"`
	TreeState  *string `json:"tree_state,omitempty"`
	CreateTime *int64  `json:"create_time,omitempty"`
}

// DiffEntry represents a single diff entry between two commits.
type DiffEntry struct {
	FileID      string  `json:"file_id"`
	FileName    string  `json:"file_name"`
	Operation   string  `json:"operation"`
	OldHash     *string `json:"old_hash,omitempty"`
	NewHash     *string `json:"new_hash,omitempty"`
	OldLocation *string `json:"old_location,omitempty"`
	NewLocation *string `json:"new_location,omitempty"`
}

// VersionEntry represents a single version in a file's version history.
type VersionEntry struct {
	CommitID   string `json:"commit_id"`
	Operation  string `json:"operation"`
	Hash       string `json:"hash"`
	CreateTime *int64 `json:"create_time,omitempty"`
	Message    string `json:"message"`
}

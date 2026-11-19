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
//

package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"ragflow/internal/utility"
)

func (c *SharePointConnector) configureFileSelection(config map[string]any) error {
	filters, err := newFileFilters(config)
	if err != nil {
		return &ConnectorValidationError{Message: err.Error()}
	}
	c.filters = filters
	c.driveID = strings.TrimSpace(stringConfig(config["drive_id"]))
	c.rootFolderIDs, err = sharePointRootIDs(config)
	if err != nil {
		return &ConnectorValidationError{Message: err.Error()}
	}
	if len(c.rootFolderIDs) > 0 && c.driveID == "" {
		return &ConnectorValidationError{Message: "drive_id is required when root_folder_ids is set"}
	}
	value, ok := config["exclude_videos"].(bool)
	if config["exclude_videos"] != nil && !ok {
		return &ConnectorValidationError{Message: "exclude_videos must be a boolean"}
	}
	c.excludeVideos = value
	return nil
}

func sharePointMetadata(drive sharePointDrive, file sharePointItem) map[string]any {
	metadata := map[string]any{
		"drive": drive.Name, "drive_id": drive.ID,
		"drive_item_id": file.ID, "path": file.relativePath,
	}
	if file.WebURL != "" {
		metadata["web_url"] = file.WebURL
	}
	return metadata
}

func (c *SharePointConnector) syncDrives(ctx context.Context, siteID string) ([]sharePointDrive, error) {
	drives, err := c.listDrives(ctx, siteID)
	if err != nil || c.driveID == "" {
		return drives, err
	}
	for _, drive := range drives {
		if drive.ID == c.driveID {
			return []sharePointDrive{drive}, nil
		}
	}
	return nil, &ConnectorValidationError{Message: "drive_id was not found in the configured SharePoint site"}
}

func (c *SharePointConnector) validateScope(ctx context.Context, siteID string) error {
	if c.driveID == "" {
		return nil
	}
	if _, err := c.syncDrives(ctx, siteID); err != nil {
		return err
	}
	for _, rootID := range c.rootFolderIDs {
		folder, err := c.getItem(ctx, c.driveID, rootID)
		if err != nil {
			return err
		}
		if folder.Folder == nil {
			return &ConnectorValidationError{Message: "root_folder_ids must identify folders in drive_id"}
		}
	}
	return nil
}

func (c *SharePointConnector) getItem(ctx context.Context, driveID, itemID string) (sharePointItem, error) {
	var item sharePointItem
	apiURL := c.graphBaseURL + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(itemID)
	err := c.getJSON(ctx, apiURL, &item)
	return item, err
}

// Pagination URLs come from the remote response. Never attach a Graph bearer
// token to a URL on a different origin or to an insecure downgrade.
func (c *SharePointConnector) validateGraphURL(rawURL string) error {
	target, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid SharePoint Graph URL")
	}
	base, err := url.Parse(c.graphBaseURL)
	if err != nil || target.Scheme != base.Scheme || !strings.EqualFold(target.Host, base.Host) || target.User != nil {
		return fmt.Errorf("SharePoint Graph URL must use the configured Graph origin")
	}
	return nil
}

func (c *SharePointConnector) skipReason(file sharePointItem) string {
	if c.filters.excludesFile(file.Name) {
		return "excluded extension"
	}
	if c.excludeVideos && (strings.HasPrefix(strings.ToLower(file.File.MimeType), "video/") || utility.IsVideoFilename(file.Name)) {
		return "excluded video"
	}
	if utility.FilenameType(file.Name) == utility.FileTypeOTHER {
		return "unsupported file type"
	}
	if file.Size > sharepointMaxFileSize {
		return "file exceeds maximum size"
	}
	return ""
}

// Content tags avoid relying on clocks or on byte size alone. Name and path
// changes also invalidate the fingerprint so indexed metadata is refreshed.
func sharePointFingerprint(file sharePointItem) string {
	tag := firstNonEmpty(file.CTag, file.ETag)
	if tag == "" {
		return ""
	}
	return stableFingerprint([]any{"sharepoint", tag, file.Size, file.Name, file.relativePath, file.WebURL})
}

type sharePointFetch struct {
	DriveID string `json:"drive_id"`
	ItemID  string `json:"item_id"`
	Tag     string `json:"tag"`
	Size    int64  `json:"size"`
}

func sharePointFetchRef(driveID string, file sharePointItem) *FetchReference {
	ref := sharePointFetch{driveID, file.ID, firstNonEmpty(file.CTag, file.ETag), file.Size}
	data, _ := json.Marshal(ref)
	return &FetchReference{Key: string(data), SizeHint: file.Size}
}

// Fetch downloads only after the runner has checked the stored fingerprint.
// Verify the revision again afterwards: never acknowledge a listed revision
// using bytes from a file that changed while it was being downloaded.
func (s *sharePointSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var file sharePointFetch
	if err := json.Unmarshal([]byte(ref.Key), &file); err != nil {
		return nil, fmt.Errorf("invalid SharePoint fetch reference")
	}
	if file.DriveID == "" || file.ItemID == "" {
		return nil, fmt.Errorf("incomplete SharePoint fetch reference")
	}
	blob, err := s.connector.downloadContent(ctx, file.DriveID, file.ItemID)
	if err != nil {
		return nil, err
	}
	if file.Tag != "" {
		if int64(len(blob)) != file.Size {
			return nil, fmt.Errorf("SharePoint download size differs from listed revision; retry synchronization")
		}
		if err := s.verifyRevision(ctx, file); err != nil {
			return nil, err
		}
	}
	return blob, nil
}

func (s *sharePointSyncSession) verifyRevision(ctx context.Context, file sharePointFetch) error {
	current, err := s.connector.getItem(ctx, file.DriveID, file.ItemID)
	if err != nil {
		return err
	}
	if firstNonEmpty(current.CTag, current.ETag) != file.Tag || current.Size != file.Size {
		return fmt.Errorf("SharePoint file changed during download; retry synchronization")
	}
	return nil
}

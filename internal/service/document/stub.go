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

//go:build !ee

package document

import (
	"context"

	"gorm.io/gorm"

	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/utility"
)

func (s *DocumentService) stopIngestionTasks(ctx context.Context, taskIDs []string) ([]*entity.IngestionTask, error) {
	userID := permission.UserIDFromContext(ctx)
	return s.ingestionTaskSvc.RequestStopMany(ctx, taskIDs, &userID)
}

// grantDocumentOwnerEE grants nothing: the community edition has no document
// permission records.
func grantDocumentOwnerEE(context.Context, *gorm.DB, *entity.Document) error { return nil }

// dedicatedParserExtension lets the enterprise edition bind a file type to its
// own built-in pipeline. The community build registers no such binding, so the
// community routing in dedicatedDocumentParserID is unchanged.
func dedicatedParserExtension(utility.FileType, string) string { return "" }

func inheritFallbackParserOptions(map[string]any, map[string]any, string) {}

// preparePreviewPresentationEE is a no-op outside the enterprise edition.
func preparePreviewPresentationEE(context.Context, *entity.Document, string) ([]byte, string, bool, error) {
	return nil, "", false, nil
}

// cleanupPreviewArtifactsEE is a no-op outside the enterprise edition.
func cleanupPreviewArtifactsEE(context.Context, *entity.Document) {}

// recordDocumentRemovalEE rows nothing: the community edition
// keeps no content history, so a removed document is not restorable.
func recordDocumentRemovalEE(context.Context, *entity.Document, *entity.Knowledgebase) error {
	return nil
}

// RestoreRemovedDocumentEE restores nothing: the community edition keeps no
// removed-document table, so a removed document cannot come back.
func RestoreRemovedDocumentEE(context.Context, string, string) (bool, error) {
	return false, nil
}

// keepFileBlobForDocumentRestore reports that nothing needs keeping: without a
// content history the blob has no reader after the delete.
func keepFileBlobForDocumentRestore() bool { return false }

// recordDocumentMetadataState records nothing: the community edition keeps no
// document metadata history, so there is nothing a restore could put back.
func recordDocumentMetadataState(_ context.Context, _ *gorm.DB, _, _ string, _ map[string]interface{}) error {
	return nil
}

// recordDocumentMetadataEditEE records nothing: the community edition keeps no
// metadata history, so there is no version to record and nothing to fail on.
func recordDocumentMetadataEditEE(_ context.Context, _ string, _ *entity.Document, _, _ map[string]interface{}) error {
	return nil
}

// recordDocumentVersionMetadata records nothing: the community edition keeps no
// versions, so there is no number to address a document's metadata by.
func recordDocumentVersionMetadata(_ context.Context, _ *gorm.DB, _ int64, _, _ string, _ map[string]interface{}) error {
	return nil
}

// parkDocumentMetadataState parks nothing: the community edition keeps no
// document states, so a creation has no first state to record.
func parkDocumentMetadataState(_ context.Context, _ []*entity.Document) {}

// ReplayedDocumentMetadata is one document's values as a restore put them back.
// The community edition restores none, so the type only keeps the reader's
// signature the same across the two builds.
type ReplayedDocumentMetadata struct {
	DocumentID string
	DatasetID  string
	Meta       map[string]interface{}
}

// RestoreDocumentMetadataAtEE restores nothing: the community edition keeps no
// document metadata history.
func RestoreDocumentMetadataAtEE(_ context.Context, _ *gorm.DB, _ string, _ int64, _ []string, _ bool) ([]ReplayedDocumentMetadata, error) {
	return nil, nil
}

// RecordReplayedDocumentStatesAtVersionEE records nothing: the community edition
// keeps no document states to address by a version.
func RecordReplayedDocumentStatesAtVersionEE(_ context.Context, _ *gorm.DB, _ int64, _ []*entity.Document, _ []ReplayedDocumentMetadata) error {
	return nil
}

// recordDocumentParserSettingStates records nothing: the community edition keeps no
// parser settings history, so there is nothing a restore could put back.
func recordDocumentParserSettingStates(_ context.Context, _ *gorm.DB, _ []*entity.Document) error {
	return nil
}

// recordDocumentVersionSettings records nothing: the community edition keeps no
// versions, so there is no number to address a document's state by.
func recordDocumentVersionSettings(_ context.Context, _ *gorm.DB, _ int64, _ []*entity.Document) error {
	return nil
}

// recordDocumentVersionSettingsTx records nothing either, for the same reason.
func recordDocumentVersionSettingsTx(_ context.Context, _ string, _ []entity.DatasetVersionChange, _ []*entity.Document) error {
	return nil
}

// RestoreParserSettingsAtEE restores nothing: the community edition keeps no
// parser settings history.
func RestoreParserSettingsAtEE(_ context.Context, _ *gorm.DB, _ string, _ int64) ([]*entity.Document, error) {
	return nil, nil
}

// parkDocumentParserSettingStates parks nothing: the community edition keeps no
// document states, and the sink it would park them in writes nothing.
func parkDocumentParserSettingStates(_ context.Context, _ *gorm.DB, _ []*entity.Document) {}

// DocumentStatusChange is one document a visibility switch moved. The community
// edition keeps no versions, so the type only keeps the shared caller's
// signature the same across the two builds.
type DocumentStatusChange struct {
	DocumentID string
	DatasetID  string
	Name       string
	Status     string
}

// RecordDocumentStatusChangesEE records nothing: the community edition keeps no
// dataset history, so a switch has no version to be recorded under.
func RecordDocumentStatusChangesEE(_ context.Context, _ *gorm.DB, _ string, _ []DocumentStatusChange) error {
	return nil
}

// RecordReplayedDocumentStatusAtVersionEE records nothing: the community edition
// keeps no document states to address by a version.
func RecordReplayedDocumentStatusAtVersionEE(_ context.Context, _ *gorm.DB, _ int64, _ []DocumentStatusChange) error {
	return nil
}

// RestoreDocumentStatusAtEE restores nothing: the community edition keeps no
// document status history.
func RestoreDocumentStatusAtEE(_ context.Context, _ *gorm.DB, _ string, _ int64) ([]DocumentStatusChange, error) {
	return nil, nil
}

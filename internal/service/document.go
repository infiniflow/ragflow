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

package service

import (
	"fmt"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// DocumentService document service
type DocumentService struct {
	documentDAO *dao.DocumentDAO
}

// NewDocumentService create document service
func NewDocumentService() *DocumentService {
	return &DocumentService{
		documentDAO: dao.NewDocumentDAO(),
	}
}

// CreateDocumentRequest create document request
type CreateDocumentRequest struct {
	Name      string `json:"name" binding:"required"`
	KbID      string `json:"kb_id" binding:"required"`
	ParserID  string `json:"parser_id" binding:"required"`
	CreatedBy string `json:"created_by" binding:"required"`
	Type      string `json:"type"`
	Source    string `json:"source"`
}

// UpdateDocumentRequest update document request
type UpdateDocumentRequest struct {
	Name        *string  `json:"name"`
	Run         *string  `json:"run"`
	TokenNum    *int64   `json:"token_num"`
	ChunkNum    *int64   `json:"chunk_num"`
	Progress    *float64 `json:"progress"`
	ProgressMsg *string  `json:"progress_msg"`
}

// DocumentResponse document response
type DocumentResponse struct {
	ID              string  `json:"id"`
	Name            *string `json:"name,omitempty"`
	KbID            string  `json:"kb_id"`
	ParserID        string  `json:"parser_id"`
	PipelineID      *string `json:"pipeline_id,omitempty"`
	Type            string  `json:"type"`
	SourceType      string  `json:"source_type"`
	CreatedBy       string  `json:"created_by"`
	Location        *string `json:"location,omitempty"`
	Size            int64   `json:"size"`
	TokenNum        int64   `json:"token_num"`
	ChunkNum        int64   `json:"chunk_num"`
	Progress        float64 `json:"progress"`
	ProgressMsg     *string `json:"progress_msg,omitempty"`
	ProcessDuration float64 `json:"process_duration"`
	Suffix          string  `json:"suffix"`
	Run             *string `json:"run,omitempty"`
	Status          *string `json:"status,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// CreateDocument create document
func (s *DocumentService) CreateDocument(req *CreateDocumentRequest) (*model.Document, error) {
	document := &model.Document{
		Name:       &req.Name,
		KbID:       req.KbID,
		ParserID:   req.ParserID,
		CreatedBy:  req.CreatedBy,
		Type:       req.Type,
		SourceType: req.Source,
		Suffix:     ".doc",
		Status:     func() *string { s := "0"; return &s }(),
	}

	if err := s.documentDAO.Create(document); err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	return document, nil
}

// GetDocumentByID get document by ID
func (s *DocumentService) GetDocumentByID(id string) (*DocumentResponse, error) {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(document), nil
}

// UpdateDocument update document
func (s *DocumentService) UpdateDocument(id string, req *UpdateDocumentRequest) error {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		document.Name = req.Name
	}
	if req.Run != nil {
		document.Run = req.Run
	}
	if req.TokenNum != nil {
		document.TokenNum = *req.TokenNum
	}
	if req.ChunkNum != nil {
		document.ChunkNum = *req.ChunkNum
	}
	if req.Progress != nil {
		document.Progress = *req.Progress
	}
	if req.ProgressMsg != nil {
		document.ProgressMsg = req.ProgressMsg
	}

	return s.documentDAO.Update(document)
}

// DeleteDocument delete document
func (s *DocumentService) DeleteDocument(id string) error {
	return s.documentDAO.Delete(id)
}

// ListDocuments list documents
func (s *DocumentService) ListDocuments(page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// GetDocumentsByAuthorID get documents by author ID
func (s *DocumentService) GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*DocumentResponse, int64, error) {
	offset := (page - 1) * pageSize
	documents, total, err := s.documentDAO.GetByAuthorID(fmt.Sprintf("%d", authorID), offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*DocumentResponse, len(documents))
	for i, doc := range documents {
		responses[i] = s.toResponse(doc)
	}

	return responses, total, nil
}

// toResponse convert model.Document to DocumentResponse
func (s *DocumentService) toResponse(doc *model.Document) *DocumentResponse {
	createdAt := time.Unix(doc.CreateTime, 0).Format("2006-01-02 15:04:05")
	updatedAt := ""
	if doc.UpdateTime != nil {
		updatedAt = time.Unix(*doc.UpdateTime, 0).Format("2006-01-02 15:04:05")
	}
	return &DocumentResponse{
		ID:              doc.ID,
		Name:            doc.Name,
		KbID:            doc.KbID,
		ParserID:        doc.ParserID,
		PipelineID:      doc.PipelineID,
		Type:            doc.Type,
		SourceType:      doc.SourceType,
		CreatedBy:       doc.CreatedBy,
		Location:        doc.Location,
		Size:            doc.Size,
		TokenNum:        doc.TokenNum,
		ChunkNum:        doc.ChunkNum,
		Progress:        doc.Progress,
		ProgressMsg:     doc.ProgressMsg,
		ProcessDuration: doc.ProcessDuration,
		Suffix:          doc.Suffix,
		Run:             doc.Run,
		Status:          doc.Status,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
}

//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package testutil

import (
	"context"
	"errors"
	"time"

	"ragflow/internal/entity"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// ──────────────────────────────────────────────────────────
// Mock DAO Implementations
// ──────────────────────────────────────────────────────────

// MockDocDAO is a mock implementation of docGetter interface.
type MockDocDAO struct {
	Docs map[string]*entity.Document
	Err  error
}

func (m *MockDocDAO) GetByID(id string) (*entity.Document, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	d, ok := m.Docs[id]
	if !ok {
		return nil, errors.New("record not found")
	}
	return d, nil
}

// MockF2dDAO is a mock implementation of f2dGetter interface.
type MockF2dDAO struct {
	Mappings map[string][]*entity.File2Document
	Err      error
}

func (m *MockF2dDAO) GetByDocumentID(docID string) ([]*entity.File2Document, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Mappings[docID], nil
}

// MockFileDAO is a mock implementation of fileGetter interface.
type MockFileDAO struct {
	Files map[string]*entity.File
	Err   error
}

func (m *MockFileDAO) GetByID(id string) (*entity.File, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	f, ok := m.Files[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return f, nil
}

// ──────────────────────────────────────────────────────────
// Mock Storage Implementation
// ──────────────────────────────────────────────────────────

// MockStorage is a complete mock implementation of storage.Storage interface.
type MockStorage struct {
	Data         map[string][]byte
	Err          error
	PutCallCount int
	PutBucket    string
	PutFnm       string
	PutData      []byte
}

// NewMockStorage creates a new MockStorage with the given data.
func NewMockStorage(data map[string][]byte) *MockStorage {
	return &MockStorage{Data: data}
}

// NewMockStorageWithError creates a new MockStorage that returns the given error.
func NewMockStorageWithError(err error) *MockStorage {
	return &MockStorage{Err: err}
}

func (m *MockStorage) Health() bool { return true }

func (m *MockStorage) Get(bucket, objectName string, tenantID ...string) ([]byte, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	key := bucket + "/" + objectName
	if len(tenantID) > 0 && tenantID[0] != "" {
		key = tenantID[0] + "/" + key
	}
	d, ok := m.Data[key]
	if !ok {
		d, ok = m.Data[objectName]
	}
	return d, nil
}

func (m *MockStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	if m.Err != nil {
		return m.Err
	}
	m.PutCallCount++
	m.PutBucket = bucket
	m.PutFnm = fnm
	m.PutData = binary
	return nil
}

func (m *MockStorage) Remove(bucket, fnm string, tenantID ...string) error { return nil }

func (m *MockStorage) ObjExist(bucket, fnm string, tenantID ...string) bool { return true }

func (m *MockStorage) GetPresignedURL(bucket, fnm string, expiry time.Duration, tenantID ...string) (string, error) {
	return "", nil
}

func (m *MockStorage) BucketExists(bucket string) bool { return true }

func (m *MockStorage) RemoveBucket(bucket string) error { return nil }

func (m *MockStorage) Copy(srcBucket, srcPath, dstBucket, dstPath string) bool { return true }

func (m *MockStorage) Move(srcBucket, srcPath, dstBucket, dstPath string) bool { return true }

// ──────────────────────────────────────────────────────────
// Mock PDF Parser Implementation
// ──────────────────────────────────────────────────────────

// MockPDFParser is a mock implementation of pdfParser interface.
type MockPDFParser struct {
	Sections []map[string]any
	Err      error
}

// NewMockPDFParser creates a new MockPDFParser.
func NewMockPDFParser(sections []map[string]any, err error) *MockPDFParser {
	return &MockPDFParser{Sections: sections, Err: err}
}

func (m *MockPDFParser) ParseWithDeepDoc(ctx context.Context, filename string, data []byte, config doctype.ParserConfig) ([]map[string]any, error) {
	return m.Sections, m.Err
}

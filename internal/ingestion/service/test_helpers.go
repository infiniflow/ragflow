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

package service

import "ragflow/internal/storage"

// ──────────────────────────────────────────────────────────
// Ingestor Test Helpers
// ──────────────────────────────────────────────────────────

// NewTestIngestor creates a new Ingestor for testing with default values.
func NewTestIngestor() *Ingestor {
	return &Ingestor{
		id: "test-ingestor",
	}
}

// IngestorOption configures a test Ingestor.
type IngestorOption func(*Ingestor)

// WithMockStorage sets the storageImpl to the given mock.
func WithMockStorage(mock storage.Storage) IngestorOption {
	return func(i *Ingestor) {
		i.storageImpl = mock
	}
}

// WithMockPDFParser sets the pdfParser to the given mock.
func WithMockPDFParser(mock pdfParser) IngestorOption {
	return func(i *Ingestor) {
		i.pdfParser = mock
	}
}

// WithMockDAOs sets the documentDAO, f2dDAO, and fileDAO.
func WithMockDAOs(docDAO docGetter, f2dDAO f2dGetter, fileDAO fileGetter) IngestorOption {
	return func(i *Ingestor) {
		i.documentDAO = docDAO
		i.f2dDAO = f2dDAO
		i.fileDAO = fileDAO
	}
}

// SetupTestIngestor creates a new test Ingestor with the given options.
func SetupTestIngestor(t testingT, opts ...IngestorOption) *Ingestor {
	t.Helper()
	i := NewTestIngestor()
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// testingT is a subset of testing.TB used by test helpers (no *testing.T import needed).
type testingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

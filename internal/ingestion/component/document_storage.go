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

package component

import (
	"context"
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// DocumentStorageRef is the resolved backing storage location for a document.
// It is exported so higher-level integration tests can inject doc_id resolution
// without reaching into DAO state.
type DocumentStorageRef struct {
	Name   string
	Bucket string
	Path   string
}

// ResolveDocumentStorageOverride is the narrow test seam for doc_id-driven
// storage resolution. Production leaves this nil and uses DAO-backed lookup.
var ResolveDocumentStorageOverride func(docID string) (*DocumentStorageRef, error)

func fetchBinary(ctx context.Context, bucket, path string) ([]byte, error) {
	stg := resolveStorage()
	if stg == nil {
		return nil, fmt.Errorf("no storage backend registered")
	}

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := stg.Get(bucket, path)
		done <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("storage.Get(%q, %q): %w", bucket, path, r.err)
		}
		return r.data, nil
	}
}

func resolveStorage() storage.Storage {
	if SetStorageFactoryOverride != nil {
		if s := SetStorageFactoryOverride(); s != nil {
			return s
		}
	}
	return storage.GetStorageFactory().GetStorage()
}

func resolveDocumentStorage(docID string) (*DocumentStorageRef, error) {
	if ResolveDocumentStorageOverride != nil {
		return ResolveDocumentStorageOverride(docID)
	}

	doc, err := dao.NewDocumentDAO().GetByID(docID)
	if err != nil {
		return nil, err
	}
	ref := &DocumentStorageRef{Name: documentNameOrID(doc)}

	mappings, err := dao.NewFile2DocumentDAO().GetByDocumentID(doc.ID)
	if err != nil {
		return nil, err
	}
	if len(mappings) > 0 && mappings[0].FileID != nil && *mappings[0].FileID != "" {
		file, err := dao.NewFileDAO().GetByID(*mappings[0].FileID)
		if err != nil {
			return nil, err
		}
		if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
			if file.Location == nil || *file.Location == "" {
				return nil, fmt.Errorf("file location is empty")
			}
			ref.Bucket = file.ParentID
			ref.Path = *file.Location
			return ref, nil
		}
	}
	if doc.Location == nil || *doc.Location == "" {
		return nil, fmt.Errorf("document location is empty")
	}
	ref.Bucket = doc.KbID
	ref.Path = *doc.Location
	return ref, nil
}

func resolveDocumentName(docID string) (string, error) {
	if ResolveDocumentStorageOverride != nil {
		ref, err := ResolveDocumentStorageOverride(docID)
		if err != nil {
			return "", err
		}
		if ref != nil && ref.Name != "" {
			return ref.Name, nil
		}
	}
	doc, err := dao.NewDocumentDAO().GetByID(docID)
	if err != nil {
		return "", err
	}
	return documentNameOrID(doc), nil
}

func documentNameOrID(doc *entity.Document) string {
	if doc != nil && doc.Name != nil && *doc.Name != "" {
		return *doc.Name
	}
	if doc != nil {
		return doc.ID
	}
	return ""
}

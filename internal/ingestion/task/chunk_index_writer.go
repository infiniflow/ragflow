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

package task

import "context"

// InsertFunc is the signature of the chunk insertion backend (e.g. engine.InsertChunks).
type InsertFunc func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error)

// chunkIndexWriter batches chunks and writes them to the search engine in
// bulkSize-sized batches. Progress is reported every 128 batches.
type chunkIndexWriter struct {
	insertFunc InsertFunc
	baseName   string
	datasetID  string
	bulkSize   int
}

// newChunkIndexWriter creates a chunkIndexWriter. When bulkSize is <= 0 the
// entire chunk slice is sent in one call.
func newChunkIndexWriter(
	insertFunc InsertFunc,
	baseName string,
	datasetID string,
	bulkSize int,
) *chunkIndexWriter {
	return &chunkIndexWriter{
		insertFunc: insertFunc,
		baseName:   baseName,
		datasetID:  datasetID,
		bulkSize:   bulkSize,
	}
}

// Write inserts chunks in batches. An empty or nil slice is forwarded to the
// backend as-is.
func (w *chunkIndexWriter) Write(ctx context.Context, chunks []map[string]any) error {
	if len(chunks) == 0 {
		_, err := w.insertFunc(ctx, chunks, w.baseName, w.datasetID)
		return err
	}
	bulkSize := w.bulkSize
	if bulkSize <= 0 {
		bulkSize = len(chunks)
	}
	for b := 0; b < len(chunks); b += bulkSize {
		end := b + bulkSize
		if end > len(chunks) {
			end = len(chunks)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := w.insertFunc(ctx, chunks[b:end], w.baseName, w.datasetID); err != nil {
			return err
		}
	}
	return nil
}

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

//go:build !ee

package dataset

import (
	"context"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func accessUserContext(ctx context.Context, _ string) (context.Context, bool) {
	return ctx, false
}

func deleteDatasetChunkRuntimeTxEE(_ context.Context, _ *gorm.DB, _ string) error {
	return nil
}

func deleteDatasetMetadataStatesEE(_ context.Context, _ string) {}

func (d *DatasetService) datasetConfigBindingsEE(_ context.Context, _ *gorm.DB, _ *entity.Knowledgebase) ([]entity.JSONMap, error) {
	return nil, nil
}

func (d *DatasetService) seedDatasetBaselineVersionTxEE(_ context.Context, _ *gorm.DB, _ *entity.Knowledgebase, _ string) error {
	return nil
}

func (d *DatasetService) recordDatasetConfigVersionTxEE(_ context.Context, _ *gorm.DB, _, _ *entity.Knowledgebase, _ []entity.JSONMap, _, _ string, _ int64) (int64, error) {
	return 0, nil
}

func (d *DatasetService) deleteDatasetVersionsTxEE(_ context.Context, _ *gorm.DB, _ string) error {
	return nil
}

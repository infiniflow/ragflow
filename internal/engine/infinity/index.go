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

package infinity

import (
	"context"
	"fmt"
)

// CreateIndex creates a table/index
func (e *infinityEngine) CreateIndex(ctx context.Context, indexName string, mapping interface{}) error {
	return fmt.Errorf("infinity create table not implemented: waiting for official Go SDK")
}

// DeleteIndex deletes a table/index
func (e *infinityEngine) DeleteIndex(ctx context.Context, indexName string) error {
	return fmt.Errorf("infinity drop table not implemented: waiting for official Go SDK")
}

// IndexExists checks if table/index exists
func (e *infinityEngine) IndexExists(ctx context.Context, indexName string) (bool, error) {
	return false, fmt.Errorf("infinity check table existence not implemented: waiting for official Go SDK")
}

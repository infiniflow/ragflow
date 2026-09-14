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

package document

import (
	"errors"
	"fmt"
	"math"

	"ragflow/internal/entity"
)

type immutableDocumentFields struct {
	chunkNum            *int64
	chunkNumRequestName string
	tokenNum            *int64
	tokenNumRequestName string
	progress            *float64
	run                 *string
	progressMsg         *string
}

func validateImmutableDocumentFields(doc *entity.Document, fields immutableDocumentFields) error {
	if fields.chunkNum != nil && *fields.chunkNum != doc.ChunkNum {
		return fmt.Errorf("can't change `%s`", fields.chunkNumRequestName)
	}
	if fields.tokenNum != nil && *fields.tokenNum != doc.TokenNum {
		return fmt.Errorf("can't change `%s`", fields.tokenNumRequestName)
	}
	if fields.progress != nil && !documentProgressEqual(*fields.progress, doc.Progress) {
		return errors.New("can't change `progress`")
	}
	if fields.run != nil && !optionalStringEqual(fields.run, doc.Run) {
		return errors.New("can't change `run`")
	}
	if fields.progressMsg != nil && !optionalStringEqual(fields.progressMsg, doc.ProgressMsg) {
		return errors.New("can't change `progress_msg`")
	}
	return nil
}

func documentProgressEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

func optionalStringEqual(a, b *string) bool {
	return a != nil && b != nil && *a == *b
}

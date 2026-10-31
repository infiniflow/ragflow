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

package parser

import (
	"testing"

	"ragflow/internal/parser/media"
)

func newMediaBudgetForTest() *media.Budget {
	return media.NewBudget()
}

func newLimitedMediaBudgetForTest(t *testing.T, maxImageBytes int, maxTotalBytes int64, maxItems int) *media.Budget {
	t.Helper()
	return media.NewBudgetWithLimits(media.Limits{
		ImageMaxBytes:   int64(maxImageBytes),
		MaxImagesPerRun: maxItems,
		MaxBytesPerRun:  maxTotalBytes,
	})
}

func includeMediaForTest(budget *media.Budget, data []byte) (included, keepWalking bool) {
	reservation, admission := budget.ReserveOccurrence(media.UnknownSize)
	if admission == media.RejectedByCount {
		return false, false
	}
	if admission != media.Admitted {
		return false, true
	}
	return reservation.Commit(media.Size(len(data))), true
}

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

package syncer

import (
	"context"
	"ragflow/internal/service"
	"time"
)

// RecoverStaleRunning restores timed-out running sync tasks to schedule.
func RecoverStaleRunning(ctx context.Context, taskService *service.SyncTaskService, now time.Time) error {
	return taskService.RecoverStaleRunning(ctx, now)
}

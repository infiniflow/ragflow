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

package admin

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

func TestPlatformSettingsCannotBeChangedAsVariables(t *testing.T) {
	s := &Service{}
	for _, key := range []string{entity.PlatformProviderLockSetting, entity.PlatformModelDefaultPrefix + "chat", "PLATFORM.PROVIDER_CREATE_LOCK", "Platform.Default_Model.Chat", "plátform.provider_create_lock"} {
		if err := s.setVariable(context.Background(), nil, key, "x"); err == nil {
			t.Errorf("reserved setting %q accepted", key)
		}
	}
}

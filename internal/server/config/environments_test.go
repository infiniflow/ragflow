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

package config

import (
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestGetEnvironmentsRejectsInvalidConnectorKey(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, "c2hvcnQ=")
	err := (&Config{}).GetEnvironments()
	if err == nil || !strings.Contains(err.Error(), common.EnvRAGFlowConnectorKey) || strings.Contains(err.Error(), "c2hvcnQ=") {
		t.Fatalf("GetEnvironments error = %v, want an error naming %s without its value", err, common.EnvRAGFlowConnectorKey)
	}
}

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

package utility

import (
	"strings"
	"testing"
)

var helperConvergenceDFiles = []string{
	"../handler/searchbot.go",
	"../handler/chat_audio.go",
	"../handler/agent_run_event.go",
	"../handler/agent_webhook.go",
	"../syncer/connector/google_cloud_storage.go",
	"../syncer/connector/s3.go",
	"../syncer/connector/google_drive.go",
	"../syncer/connector/azure_blob.go",
	"../syncer/connector/oci_storage.go",
	"../syncer/connector/box.go",
	"../deepdoc/parser/pdf/tool/config.go",
}

// D-family inline forms converged to utility.MustJSON / common.GetEnvInt.
// Sites that intentionally handle a marshal error are out of scope.
var helperConvergenceDPatterns = map[string]string{
	"../handler/searchbot.go":                     "b, _ := json.Marshal(payload)",
	"../handler/chat_audio.go":                    "data, _ := json.Marshal(",
	"../handler/agent_run_event.go":               "data, _ := json.Marshal(map[string]string{",
	"../handler/agent_webhook.go":                 "encoded, _ := json.Marshal(probe)",
	"../syncer/connector/google_cloud_storage.go": "fetchKey, _ := json.Marshal(fetch)",
	"../syncer/connector/s3.go":                   "fetchKey, _ := json.Marshal(fetch)",
	"../syncer/connector/google_drive.go":         "fetchKey, _ := json.Marshal(fetch)",
	"../syncer/connector/azure_blob.go":           "fetchKey, _ := json.Marshal(fetch)",
	"../syncer/connector/oci_storage.go":          "fetchKey, _ := json.Marshal(fetch)",
	"../syncer/connector/box.go":                  "fetchKey, _ := json.Marshal(",
	"../deepdoc/parser/pdf/tool/config.go":        "func envInt(key string, def int) int {\n\tv := common.GetEnv(key)",
}

func TestMustJSONAndEnvConverged(t *testing.T) {
	leftovers := make([]string, 0)
	for path, pattern := range helperConvergenceDPatterns {
		if strings.Contains(readHelperConvergenceAFile(t, path), pattern) {
			leftovers = append(leftovers, path+": "+pattern)
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("inline marshal/env forms still present:\n  %s", strings.Join(leftovers, "\n  "))
	}

	mustJSONCalls := 0
	for _, path := range helperConvergenceDFiles {
		mustJSONCalls += strings.Count(readHelperConvergenceAFile(t, path), "utility.MustJSON(")
	}
	if mustJSONCalls < 15 {
		t.Errorf("expected at least 15 utility.MustJSON call sites, found %d", mustJSONCalls)
	}
}

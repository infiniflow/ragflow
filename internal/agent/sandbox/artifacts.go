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

// artifacts.go holds the cross-provider allowlist of artifact
// extensions. LocalProvider and SSHProvider share this set so
// an artifact accepted by one is accepted by the other — the
// model sees a uniform `artifacts: [{name, content_b64,
// mime_type, size}, ...]` shape across providers.
//
// The set is ported verbatim from the Python side:
//
//	agent/sandbox/providers/local.py  (the _collect_artifacts
//	    allowlist)
//	agent/sandbox/providers/ssh.py    (ALLOWED_ARTIFACT_EXTENSIONS)
//
// Python's allowlist includes `.csv .html .jpeg .jpg .json .pdf
// .png .svg`. We mirror it 1:1. To extend, add here AND in
// both Python files.

package sandbox

// allowedArtifactExts is the set of file extensions the Local
// and SSH providers accept as code-execution artifacts. Anything
// outside this set is rejected at collect time.
var allowedArtifactExts = map[string]struct{}{
	".csv":  {},
	".html": {},
	".jpeg": {},
	".jpg":  {},
	".json": {},
	".pdf":  {},
	".png":  {},
	".svg":  {},
}

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

package office

import "errors"

// ErrOfficeCGORequired is returned by OpenAndExtract when the build has no
// native office_oxide backend. The parser package wraps it into its own
// ErrOfficeCGORequired so callers observe a stable sentinel regardless of
// which layer reports the missing engine.
var ErrOfficeCGORequired = errors.New("deepdoc/docx: cgo required")

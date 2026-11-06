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

// Package template provides the built-in ingestion template resources.
package template

import (
	"embed"
	"io/fs"
)

//go:embed ingestion_pipeline_*.json
var files embed.FS

// FS exposes the shared, read-only source used by execution and catalog seeding.
func FS() fs.FS {
	return files
}

//go:build desktop

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

package webui

import (
	"embed"
	"io/fs"
)

// dist is a copy of web/dist that build.sh puts here before the Go link: go:embed
// cannot reach outside the package directory, and the bundle is a build product
// rather than source, so the copy stays out of version control.
//
//go:embed all:dist
var dist embed.FS

// Available reports that this binary carries a frontend bundle.
func Available() bool { return true }

func assets() (fs.FS, error) { return fs.Sub(dist, "dist") }

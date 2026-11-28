//go:build !desktop

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

package main

import (
	"context"

	"ragflow/internal/server/bootstrap"
)

// startDesktopBackends is a no-op in the server build: every backend is a
// service the operator configured, so nothing starts here. The returned stop
// function is never nil, matching the desktop build so main() can defer it
// unconditionally.
func startDesktopBackends(context.Context, *bootstrap.ServerArgs) (func(), error) {
	return func() {}, nil
}

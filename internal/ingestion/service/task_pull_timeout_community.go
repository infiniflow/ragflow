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

//go:build !ee

package service

import "time"

// taskPullRequestTimeout bounds a single-queue drain. The community build has
// exactly one lane (the bare tasks.RAGFLOW subject), so one 1s fetch window is
// enough; the outer budget need not cover the enterprise 3-lane probe.
const taskPullRequestTimeout = 1 * time.Second

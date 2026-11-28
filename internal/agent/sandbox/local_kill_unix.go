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

//go:build !windows

package sandbox

import (
	"errors"
	"syscall"
)

// killProcessTree sends SIGKILL to the process group of pid, which
// is the group localSysProcAttr put the child in. The negative pid
// means "every process in the group", so descendants die with it.
func killProcessTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}

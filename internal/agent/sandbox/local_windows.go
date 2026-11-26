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

//go:build windows

package sandbox

import "syscall"

// createNewProcessGroup is CREATE_NEW_PROCESS_GROUP: it makes the
// child the root of its own console process group, which is what
// taskkill /T needs to walk the descendants (see killProcessTree).
const createNewProcessGroup = 0x00000200

// localSysProcAttr starts the child in its own process group so the
// whole tree can be killed on timeout. Windows has no Pdeathsig
// equivalent; a child can outlive an unexpected parent exit.
func localSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

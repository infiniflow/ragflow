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

import (
	"errors"
	"os/exec"
	"strconv"
)

// killProcessTree terminates pid and its descendants.
//
// Windows has no process groups to signal: the child started with
// CREATE_NEW_PROCESS_GROUP is only a console group, and nothing
// there is delivered as a signal. `taskkill /T` walks the parent
// chain and force-terminates the tree, which is the platform's
// equivalent of the POSIX process-group kill. A child that already
// exited makes taskkill fail; that is not an error for the caller,
// which only escalates after a timeout or cancellation.
func killProcessTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

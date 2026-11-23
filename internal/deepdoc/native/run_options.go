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

//go:build cgo

package native

import (
	"context"
	"sync"

	ort "github.com/infiniflow/onnxruntime_go"
)

// watchRunOptions terminates opts when ctx is cancelled and returns a stop
// function that joins the watcher. Callers must defer the returned function
// after the deferred opts.Destroy(): defers run LIFO, so the join executes
// first and Terminate() can never touch a freed RunOptions handle.
func watchRunOptions(ctx context.Context, opts *ort.RunOptions) func() {
	done := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		select {
		case <-ctx.Done():
			_ = opts.Terminate()
		case <-done:
		}
	}()
	return func() {
		close(done)
		watcher.Wait()
	}
}

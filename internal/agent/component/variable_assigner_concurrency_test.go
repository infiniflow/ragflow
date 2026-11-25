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

package component

import (
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestVariableAssignerConcurrentStateWrites(t *testing.T) {
	state := runtime.NewCanvasState("race", "race")
	refs := []string{"sys.shared", "env.shared", "item", "index"}
	const workers = 16
	const writes = 500
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		ref := refs[worker%len(refs)]
		wg.Add(1)
		go func(worker int, ref string) {
			defer wg.Done()
			<-start
			for i := 0; i < writes; i++ {
				if err := writeVar(state, ref, i); err != nil {
					t.Errorf("writeVar(%q): %v", ref, err)
					return
				}
				switch ref {
				case "sys.shared":
					state.GetSys("shared")
				case "env.shared":
					state.GetEnv("shared")
				case "item":
					state.GetGlobal("__item__")
				case "index":
					state.GetGlobal("__index__")
				}
			}
		}(worker, ref)
	}
	close(start)
	wg.Wait()
}

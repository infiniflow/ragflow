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

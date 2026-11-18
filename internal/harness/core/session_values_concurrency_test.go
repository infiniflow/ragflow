package core

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestForkRunCtxSharedValuesConcurrentAccess(t *testing.T) {
	parent := setRunCtx(t.Context(), &runContext{Session: newRunSession()})
	const workers = 8
	const writes = 500
	contexts := make([]context.Context, workers)
	for i := range contexts {
		contexts[i] = forkRunCtx(parent)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker, ctx := range contexts {
		wg.Add(1)
		go func(worker int, ctx context.Context) {
			defer wg.Done()
			<-start
			for i := 0; i < writes; i++ {
				if err := SetRunLocalValue(ctx, "shared", fmt.Sprintf("%d-%d", worker, i)); err != nil {
					t.Errorf("SetRunLocalValue: %v", err)
					return
				}
				if _, _, err := GetRunLocalValue(ctx, "shared"); err != nil {
					t.Errorf("GetRunLocalValue: %v", err)
					return
				}
				_ = resolveTemplate("{shared}", ctx)
				setOutputToSession(ctx, &schema.Message{Content: fmt.Sprintf("%d-%d", worker, i)}, "output")
				if i%2 == 0 {
					if err := DeleteRunLocalValue(ctx, "shared"); err != nil {
						t.Errorf("DeleteRunLocalValue: %v", err)
						return
					}
				}
			}
		}(worker, ctx)
	}
	close(start)
	wg.Wait()

	if _, ok, err := GetRunLocalValue(parent, "output"); err != nil || !ok {
		t.Fatalf("shared output value missing: ok=%v err=%v", ok, err)
	}
}

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

package utility

import (
	"context"
	"testing"
	"time"
)

func TestWorkerPoolIdleShrinkGrowDoesNotOverspawn(t *testing.T) {
	p := NewWorkerPool[int, int](4, 10, func(context.Context, int) (int, error) { return 1, nil })
	defer p.StopWait()
	deadline := time.Now().Add(time.Second)
	for p.Stats().LiveWorkers < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p.Resize(1)
	p.Resize(3)
	time.Sleep(20 * time.Millisecond)
	if n := p.Stats().LiveWorkers; n > 4 {
		t.Fatalf("idle shrink/grow overspawned: %d workers, want at most initial 4", n)
	}
}

func TestWorkerPoolShrinksThenRunsQueuedWork(t *testing.T) {
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	p := NewWorkerPool[int, int](4, 10, func(_ context.Context, n int) (int, error) {
		if n < 4 {
			entered <- struct{}{}
			<-release
		}
		return n, nil
	})
	var futures []WorkerPoolFuture[int, int]
	for i := 0; i < 4; i++ {
		f, _ := p.Submit(context.Background(), i)
		futures = append(futures, f)
	}
	for i := 0; i < 4; i++ {
		<-entered
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := 4; i < 12; i++ {
		f, err := p.Submit(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		futures = append(futures, f)
	}
	p.Resize(1)
	close(release)
	for _, f := range futures {
		if _, err := f.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for p.Stats().LiveWorkers != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := p.Stats().LiveWorkers; n != 1 {
		t.Fatalf("shrink left %d workers, want 1", n)
	}
	p.StopWait()
}

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

package tokenizer

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
)

// prepareCloseRaceTest pins the engine type for the test, restores it
// afterwards, and skips when the analyzer dictionary is not present. The
// dictionary is a local asset rather than a service, but the unit tier must
// still run on a machine that has not downloaded it.
func prepareCloseRaceTest(t *testing.T) {
	t.Helper()

	original := engineType
	engineType = ""
	t.Cleanup(func() { engineType = original })

	dir := os.Getenv(common.EnvRAGFlowDictPath)
	if dir == "" {
		dir = "/usr/share/infinity/resource"
	}
	if _, err := os.Stat(filepath.Join(dir, "rag", "huqie.trie")); err != nil {
		t.Skipf("analyzer dictionary not present at %s (%v); run deps/download_deps.py", dir, err)
	}
}

// smallPoolConfig keeps the pool at one pre-warmed slot with a generous acquire
// timeout, so an acquire that cannot be served parks on the blocking path
// instead of timing out.
func smallPoolConfig() *PoolConfig {
	return &PoolConfig{
		DictPath:       "",
		MinSize:        1,
		MaxSize:        1,
		IdleTimeout:    5 * time.Second,
		AcquireTimeout: 30 * time.Second,
	}
}

// waitPoolClosed blocks until p reports not initialized, so a caller can release
// a checked-out instance after Close has flipped the pool but while Close is
// still waiting for that release.
func waitPoolClosed(t *testing.T, p *analyzerPool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for p.isInitialized() {
		if time.Now().After(deadline) {
			t.Fatal("Close did not flip the pool to closed within 2s")
		}
		time.Sleep(time.Millisecond)
	}
}

// A blocked acquire must wake up and return an error when Close runs, Close must
// wait for the checked-out instances, and it must return promptly once the last
// one is released. On the unsynchronized pool, close(instances) turns the parked
// receive into a nil instance and panics on the next field write, and the late
// release sends on the closed channel.
func TestAcquireUnblocksOnCloseCompletion(t *testing.T) {
	prepareCloseRaceTest(t)

	if err := Init(smallPoolConfig()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	p := globalPool.Load()
	if p == nil {
		t.Fatal("globalPool is nil after Init")
	}

	// Hold the only slot so the next acquire is forced onto the blocking path.
	held, err := p.acquire()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := p.acquire()
		done <- err
	}()

	<-started
	// The pool is empty and at MaxSize, so the worker exhausts the fast path and
	// the dynamic-expansion branch and parks in the timeout select. Give it a
	// moment to get there before Close.
	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan time.Duration, 1)
	go func() {
		begin := time.Now()
		p.Close()
		closeDone <- time.Since(begin)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("parked acquire returned a nil error after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parked acquire did not wake within 2s of Close")
	}

	// Close is waiting for this instance; releasing it must not send on the
	// closed channel and must let Close finish.
	p.release(held)

	select {
	case elapsed := <-closeDone:
		if elapsed > 2*time.Second {
			t.Fatalf("Close took %v after the last borrower was released", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the last borrower was released")
	}
}

// Releasing a checked-out instance after Close has flipped the pool must close
// that instance rather than send on the closed channel.
func TestReleaseAfterCloseSelfClosesInstance(t *testing.T) {
	prepareCloseRaceTest(t)

	if err := Init(smallPoolConfig()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	p := globalPool.Load()
	held, err := p.acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	waitPoolClosed(t, p)

	p.release(held) // must not panic
	<-closeDone
}

// Concurrent Close calls must close the underlying channel exactly once and all
// return once the last borrower is released.
func TestConcurrentCloseIsIdempotent(t *testing.T) {
	prepareCloseRaceTest(t)

	if err := Init(&PoolConfig{
		DictPath:       "",
		MinSize:        1,
		MaxSize:        4,
		IdleTimeout:    5 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	p := globalPool.Load()
	held, err := p.acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	const closers = 8
	var wg sync.WaitGroup
	wg.Add(closers)
	for i := 0; i < closers; i++ {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("concurrent Close panicked: %v", r)
				}
			}()
			p.Close()
		}()
	}

	waitPoolClosed(t, p)
	p.release(held)
	wg.Wait()

	if IsInitialized() {
		t.Error("pool still reports initialized after concurrent Close")
	}
}

// Init must be able to rebuild a pool that a previous Close shut down.
func TestInitAfterCloseRebuildsPool(t *testing.T) {
	prepareCloseRaceTest(t)

	if err := Init(smallPoolConfig()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !IsInitialized() {
		t.Fatal("pool not initialized after Init")
	}

	Close()
	if IsInitialized() {
		t.Fatal("pool still reports initialized after Close")
	}

	if err := Init(smallPoolConfig()); err != nil {
		t.Fatalf("re-Init after Close: %v", err)
	}
	defer Close()

	if !IsInitialized() {
		t.Fatal("pool not initialized after re-Init")
	}
	if _, err := New("").Tokenize("hello world"); err != nil {
		t.Fatalf("Tokenize after re-Init: %v", err)
	}
}

// Hammer tokenization from many goroutines while Close runs. Every error is
// tolerated; the assertions are that nothing panics and, under -race, that the
// lifecycle state is not read and written concurrently. Close must also wait for
// the in-flight tokenizations before it destroys the shared dictionaries.
func TestTokenizeCloseRaceStress(t *testing.T) {
	prepareCloseRaceTest(t)

	if err := Init(&PoolConfig{
		DictPath:       "",
		MinSize:        2,
		MaxSize:        8,
		IdleTimeout:    50 * time.Millisecond,
		AcquireTimeout: 100 * time.Millisecond,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	const workers = 16
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Tokenize panicked during Close: %v", r)
				}
			}()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = New("").Tokenize("hello world 测试")
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)
	// Close while the workers are still hammering the pool. Close must wait for
	// the in-flight tokenizations to return their instances before it destroys
	// the base analyzer whose dictionaries they share.
	Close()
	close(stop)
	wg.Wait()

	// The pool must be usable again after the close storm.
	if err := Init(smallPoolConfig()); err != nil {
		t.Fatalf("re-Init after close storm: %v", err)
	}
	Close()
}

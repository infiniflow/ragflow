package core

import (
	"testing"
	"time"
)

func TestAsyncIteratorCloseUnblocksGeneratorSend(t *testing.T) {
	iter, gen := NewAsyncIteratorPair[int]()
	for i := 0; i < cap(iter.ch); i++ {
		gen.Send(i)
	}

	sent := make(chan struct{})
	go func() {
		gen.Send(cap(iter.ch))
		close(sent)
	}()
	iter.Close()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("generator send remained blocked after iterator close")
	}
	gen.Close()
}

func TestAsyncGeneratorCloseDrainsBufferedItems(t *testing.T) {
	iter, gen := NewAsyncIteratorPair[int]()
	gen.Send(1)
	gen.Send(2)
	gen.Close()

	for _, want := range []int{1, 2} {
		got, ok := iter.Next()
		if !ok || got != want {
			t.Fatalf("Next() = (%d, %t), want (%d, true)", got, ok, want)
		}
	}
	if got, ok := iter.Next(); ok {
		t.Fatalf("Next() after draining closed generator = (%d, true), want no item", got)
	}
}

func TestAsyncGeneratorCloseUnblocksBlockedSend(t *testing.T) {
	iter, gen := NewAsyncIteratorPair[int]()
	for i := 0; i < cap(iter.ch); i++ {
		gen.Send(i)
	}

	sent := make(chan struct{})
	go func() {
		gen.Send(cap(iter.ch))
		close(sent)
	}()
	closed := make(chan struct{})
	go func() {
		gen.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("generator close remained blocked on an in-flight send")
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("in-flight send did not stop after generator close")
	}
	iter.Close()
}

func TestAsyncGeneratorSendAfterIteratorCloseIsIgnored(t *testing.T) {
	iter, gen := NewAsyncIteratorPair[int]()
	iter.Close()
	if !gen.IsClosed() {
		t.Fatal("generator should report a closed stream after iterator close")
	}
	gen.Send(1)
	if gen.trySend(2) {
		t.Fatal("trySend succeeded after the iterator was closed")
	}
	if gen.SendCtx(t.Context(), 3) {
		t.Fatal("SendCtx succeeded after the iterator was closed")
	}
	gen.Close()
}

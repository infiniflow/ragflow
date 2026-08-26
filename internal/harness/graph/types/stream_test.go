// Package stream provides tests for the stream protocol.
package types

import (
	"context"
	"testing"
	"time"
)

func TestChannelStream_BasicOperations(t *testing.T) {
	ctx := context.Background()
	stream := NewChannelStream(StreamModeValues, 10)

	// Test Emit and Iterator
	chunk := &StreamChunk{
		Mode:      StreamModeValues,
		Data:      map[string]interface{}{"key": "value"},
		Step:      0,
		Node:      "test",
		Timestamp: time.Now().Unix(),
	}

	if err := stream.Emit(ctx, chunk); err != nil {
		t.Errorf("Emit failed: %v", err)
	}

	iterator := stream.Iterator(ctx)
	if iterator == nil {
		t.Error("Iterator should not be nil")
	}

	received, err := iterator.Next(ctx)
	if err != nil {
		t.Errorf("Next failed: %v", err)
	}
	if received == nil {
		t.Error("Should receive a chunk")
	}
	if received.Step != 0 {
		t.Errorf("Expected step 0, got %d", received.Step)
	}

	// Close stream
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !stream.IsClosed() {
		t.Error("Stream should be closed")
	}
}

func TestChannelStream_MultipleChunks(t *testing.T) {
	ctx := context.Background()
	stream := NewChannelStream(StreamModeUpdates, 10)

	// Emit multiple chunks
	for i := 0; i < 5; i++ {
		chunk := &StreamChunk{
			Mode: StreamModeUpdates,
			Data: map[string]interface{}{"count": i},
			Step: i,
		}
		if err := stream.Emit(ctx, chunk); err != nil {
			t.Errorf("Emit failed at iteration %d: %v", i, err)
		}
	}

	// Close to signal no more chunks
	stream.Close()

	// Read all chunks
	iterator := stream.Iterator(ctx)
	count := 0
	for iterator.HasMore() {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			break
		}
		if chunk.Step != count {
			t.Errorf("Expected step %d, got %d", count, chunk.Step)
		}
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 chunks, received %d", count)
	}
}

func TestDuplexStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create source streams
	stream1 := NewChannelStream(StreamModeValues, 10)
	stream2 := NewChannelStream(StreamModeUpdates, 10)

	// Create duplex stream
	duplex := NewDuplexStream(ctx, stream1, stream2)

	// Emit to source streams
	go func() {
		stream1.Emit(ctx, &StreamChunk{Mode: StreamModeValues, Data: "from1", Step: 0})
		stream1.Close()
	}()

	go func() {
		stream2.Emit(ctx, &StreamChunk{Mode: StreamModeUpdates, Data: "from2", Step: 1})
		stream2.Close()
	}()

	// Give time for multiplexing
	time.Sleep(100 * time.Millisecond)

	// Close duplex
	duplex.Close()

	// Read from output
	iterator := duplex.Output().Iterator(ctx)
	count := 0
	for iterator.HasMore() {
		_, err := iterator.Next(ctx)
		if err != nil {
			break
		}
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 chunks from duplex, got %d", count)
	}
}

func TestFilterStream(t *testing.T) {
	ctx := context.Background()
	source := NewChannelStream(StreamModeValues, 10)

	// Create filter that only allows chunks with even step
	filter := NewFilterStream(source, func(chunk *StreamChunk) bool {
		return chunk.Step%2 == 0
	})

	// Emit chunks
	for i := 0; i < 5; i++ {
		source.Emit(ctx, &StreamChunk{Mode: StreamModeValues, Step: i, Data: i})
	}
	source.Close()

	// Read filtered chunks
	count := 0
	iterator := filter.Iterator(ctx)
	for iterator.HasMore() {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			break
		}
		if chunk.Step%2 != 0 {
			t.Errorf("Filtered chunk should have even step, got %d", chunk.Step)
		}
		count++
	}

	if count != 3 { // Steps 0, 2, 4
		t.Errorf("Expected 3 filtered chunks, got %d", count)
	}
}

func TestMapStream(t *testing.T) {
	ctx := context.Background()
	source := NewChannelStream(StreamModeValues, 10)

	// Create mapper that doubles the step
	mapper := NewMapStream(source, func(chunk *StreamChunk) *StreamChunk {
		return &StreamChunk{
			Mode:      chunk.Mode,
			Data:      chunk.Data,
			Step:      chunk.Step * 2,
			Node:      chunk.Node,
			Metadata:  chunk.Metadata,
			Timestamp: chunk.Timestamp,
			Index:     chunk.Index,
		}
	})

	// Emit chunk
	source.Emit(ctx, &StreamChunk{Mode: StreamModeValues, Step: 5, Data: "test"})
	source.Close()

	// Read mapped chunk
	iterator := mapper.Iterator(ctx)
	if iterator.HasMore() {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			t.Errorf("Next failed: %v", err)
		}
		if chunk.Step != 10 {
			t.Errorf("Expected step 10 (doubled), got %d", chunk.Step)
		}
	}
}

func TestStreamMode_String(t *testing.T) {
	tests := []struct {
		mode     StreamMode
		expected string
	}{
		{StreamModeValues, "values"},
		{StreamModeUpdates, "updates"},
		{StreamModeCheckpoints, "checkpoints"},
		{StreamModeTasks, "tasks"},
		{StreamModeDebug, "debug"},
		{StreamModeMessages, "messages"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("StreamMode %v: expected %s, got %s", tt.mode, tt.expected, string(tt.mode))
		}
	}
}

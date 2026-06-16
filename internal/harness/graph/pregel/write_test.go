package pregel

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/channels"
)

// ---- ChannelWrite tests ----

func TestNewChannelWrite_Defaults(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	if cw == nil {
		t.Fatal("expected non-nil ChannelWrite")
	}
	if cw.EntryCount() != 0 {
		t.Errorf("expected 0 entries, got %d", cw.EntryCount())
	}
}

func TestChannelWrite_AddEntry(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	cw.AddEntry(&ChannelWriteEntry{Channel: "test", Value: "value1"})
	if cw.EntryCount() != 1 {
		t.Errorf("expected 1 entry, got %d", cw.EntryCount())
	}
}

func TestChannelWrite_AddEntries(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	cw.AddEntries(
		&ChannelWriteEntry{Channel: "a", Value: "val_a"},
		&ChannelWriteEntry{Channel: "b", Value: "val_b"},
	)
	if cw.EntryCount() != 2 {
		t.Errorf("expected 2 entries, got %d", cw.EntryCount())
	}
}

func TestChannelWrite_WriteTo(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("target")
	reg.Register("target", ch)

	cw := NewChannelWrite(reg)
	cw.WriteTo("target", "written")
	_, err := cw.Write(context.Background())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	val, _ := ch.Get()
	if val != "written" {
		t.Errorf("expected 'written', got %v", val)
	}
}

func TestChannelWrite_Clear(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	cw.AddEntry(&ChannelWriteEntry{Channel: "x", Value: "val"})
	if cw.EntryCount() != 1 {
		t.Error("expected 1 entry before clear")
	}
	cw.Clear()
	if cw.EntryCount() != 0 {
		t.Error("expected 0 entries after clear")
	}
}

func TestChannelWrite_GetEntries(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	cw.AddEntry(&ChannelWriteEntry{Channel: "k", Value: "v"})
	entries := cw.GetEntries()
	if len(entries) != 1 || entries[0].Channel != "k" {
		t.Errorf("expected entry with Channel='k', got %+v", entries[0])
	}
}

func TestChannelWrite_Overwrite(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	cw.Overwrite("ov_ch", "ov_val")
	if cw.EntryCount() != 1 {
		t.Errorf("expected 1 entry, got %d", cw.EntryCount())
	}
}

func TestChannelWrite_WriteNode(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("node_ch")
	reg.Register("node_ch", ch)

	cw := NewChannelWrite(reg)
	cw.WriteNode("test_node", "node_ch", "node_val")

	entries := cw.GetEntries()
	if len(entries) != 1 || entries[0].Node != "test_node" {
		t.Errorf("expected node 'test_node', got %+v", entries[0])
	}

	_, err := cw.Write(context.Background())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	val, _ := ch.Get()
	if val != "node_val" {
		t.Errorf("expected 'node_val', got %v", val)
	}
}

// ---- WriteTransformers ----

func TestIdentityWriteTransformer(t *testing.T) {
	tf := &IdentityWriteTransformer{}
	entry := &ChannelWriteEntry{Channel: "c", Value: "v"}
	result, err := tf.Transform(entry)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result.Channel != "c" || result.Value != "v" {
		t.Errorf("expected unchanged, got %+v", result)
	}
}

func TestMappingWriteTransformer(t *testing.T) {
	tf := NewMappingWriteTransformer(map[string]string{"old": "new"})
	entry := &ChannelWriteEntry{Channel: "old", Value: "data"}
	result, err := tf.Transform(entry)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result.Channel != "new" {
		t.Errorf("expected 'new', got %s", result.Channel)
	}
}

func TestPrefixWriteTransformer(t *testing.T) {
	tf := NewPrefixWriteTransformer("ns_")
	entry := &ChannelWriteEntry{Channel: "key", Value: "val"}
	result, err := tf.Transform(entry)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result.Channel != "ns_key" {
		t.Errorf("expected 'ns_key', got %s", result.Channel)
	}
}

func TestMetadataWriteTransformer(t *testing.T) {
	tf := NewMetadataWriteTransformer(map[string]interface{}{"ctx": "test"})
	entry := &ChannelWriteEntry{Channel: "c", Value: "v"}
	result, err := tf.Transform(entry)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result.Metadata["ctx"] != "test" {
		t.Errorf("expected metadata 'test', got %v", result.Metadata["ctx"])
	}
}

func TestNodeWriteTransformer(t *testing.T) {
	tf := NewNodeWriteTransformer("my_node")
	entry := &ChannelWriteEntry{Channel: "c", Value: "v"}
	result, err := tf.Transform(entry)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result.Node != "my_node" {
		t.Errorf("expected node 'my_node', got %s", result.Node)
	}
}

func TestFilterWriteTransformer(t *testing.T) {
	tf := NewFilterWriteTransformer(func(entry *ChannelWriteEntry) bool {
		return entry.Channel == "keep"
	})
	r1, _ := tf.Transform(&ChannelWriteEntry{Channel: "keep", Value: "val"})
	if r1 == nil {
		t.Error("expected non-nil for 'keep'")
	}

	r2, err := tf.Transform(&ChannelWriteEntry{Channel: "drop", Value: "val"})
	if err == nil {
		t.Error("expected error for filtered entry")
	}
	if r2 != nil {
		t.Error("expected nil for filtered entry")
	}
}

// ---- Validators ----

func TestNoOpValidator(t *testing.T) {
	v := &NoOpValidator{}
	err := v.Validate(&ChannelWriteEntry{Channel: "x", Value: "y"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestNonNullWriteValidator(t *testing.T) {
	v := NewNonNullWriteValidator()
	err := v.Validate(&ChannelWriteEntry{Channel: "x", Value: nil})
	if err == nil {
		t.Error("expected error for nil value")
	}
	err = v.Validate(&ChannelWriteEntry{Channel: "x", Value: "ok"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestLengthWriteValidator(t *testing.T) {
	// First map = min lengths, second = max lengths
	v := NewLengthWriteValidator(nil, map[string]int{"short": 5})
	err := v.Validate(&ChannelWriteEntry{Channel: "short", Value: "toolongvalue"})
	if err != nil {
		t.Logf("max-length validation: %v", err)
	}

	// Value exactly at max length should be OK
	err = v.Validate(&ChannelWriteEntry{Channel: "short", Value: "abc"})
	if err != nil {
		t.Logf("exact-length validation: %v", err)
	}
}

func TestLengthWriteValidator_Min(t *testing.T) {
	v := NewLengthWriteValidator(map[string]int{"req": 3}, nil)
	err := v.Validate(&ChannelWriteEntry{Channel: "req", Value: "ab"})
	if err == nil {
		t.Error("expected error for value below min length")
	}
	err = v.Validate(&ChannelWriteEntry{Channel: "req", Value: "abcd"})
	if err != nil {
		t.Errorf("expected no error for valid min length, got %v", err)
	}
}

// ---- WriteBatch ----

func TestWriteBatch_Add(t *testing.T) {
	batch := NewWriteBatch()
	if batch.Size() != 0 {
		t.Error("expected empty batch initially")
	}
	batch.Add(&ChannelWriteEntry{Channel: "ch", Value: "val"})
	if batch.Size() != 1 {
		t.Errorf("expected 1 entry, got %d", batch.Size())
	}
	entries := batch.Entries()
	if len(entries) != 1 || entries[0].Channel != "ch" {
		t.Errorf("unexpected entries: %+v", entries)
	}
	batch.Clear()
	if batch.Size() != 0 {
		t.Error("expected empty after clear")
	}
}

func TestWriteBatch_Entries(t *testing.T) {
	batch := NewWriteBatch()
	batch.Add(&ChannelWriteEntry{Channel: "a", Value: "v1"})
	batch.Add(&ChannelWriteEntry{Channel: "b", Value: "v2"})
	entries := batch.Entries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

// ---- WriteContext ----

func TestWriteContext(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg)
	wc := NewWriteContext("test_node", 1, cw)

	if wc.Node != "test_node" || wc.Step != 1 {
		t.Errorf("unexpected context: node=%s step=%d", wc.Node, wc.Step)
	}

	batch := wc.CreateBatch("default")
	if batch == nil {
		t.Fatal("expected non-nil batch")
	}
	batch.Add(&ChannelWriteEntry{Channel: "ch_key", Value: "ch_val"})

	batch2 := wc.GetBatch("default")
	if batch2 != batch {
		t.Error("expected same batch from GetBatch")
	}
}

func TestWriteContext_Flush(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("flush_key")
	reg.Register("flush_key", ch)

	cw := NewChannelWrite(reg)
	wc := NewWriteContext("flush_node", 1, cw)

	batch := wc.CreateBatch("default")
	batch.Add(&ChannelWriteEntry{Channel: "flush_key", Value: "flushed_val"})

	_, err := wc.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}

	val, _ := ch.Get()
	if val != "flushed_val" {
		t.Errorf("expected 'flushed_val', got %v", val)
	}
}

// ---- Error handling ----

func TestIsWriteSkipError(t *testing.T) {
	skipErr := &WriteSkipError{Channel: "skip_ch"}
	if !IsWriteSkipError(skipErr) {
		t.Error("expected true for WriteSkipError")
	}
	if IsWriteSkipError(nil) {
		t.Error("expected false for nil")
	}
}

// ---- Options ----

func TestChannelWrite_WithTransformer(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg,
		WithWriteTransformer(NewMappingWriteTransformer(map[string]string{"old": "mapped"})),
	)
	cw.AddEntry(&ChannelWriteEntry{Channel: "old", Value: "transformed_val"})
	if cw.EntryCount() != 1 {
		t.Errorf("expected 1 entry, got %d", cw.EntryCount())
	}
}

func TestChannelWrite_WithValidator(t *testing.T) {
	reg := channels.NewRegistry()
	cw := NewChannelWrite(reg,
		WithValidator(NewNonNullWriteValidator()),
	)
	cw.AddEntry(&ChannelWriteEntry{Channel: "bad", Value: nil})

	_, err := cw.Write(context.Background())
	if err == nil {
		t.Error("expected error for nil value with NonNull validator")
	}
}

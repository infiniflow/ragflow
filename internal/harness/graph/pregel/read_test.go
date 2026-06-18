package pregel

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/channels"
)

// ---- ChannelRead tests ----

// TestNewChannelRead_Defaults verifies default selector and transformer.
func TestNewChannelRead_Defaults(t *testing.T) {
	reg := channels.NewRegistry()
	cr := NewChannelRead(reg)
	if cr == nil {
		t.Fatal("expected non-nil ChannelRead")
	}

	_, ok := cr.selector.(*AllChannelsSelector)
	if !ok {
		t.Error("expected default AllChannelsSelector")
	}

	_, ok = cr.transformer.(*IdentityTransformer)
	if !ok {
		t.Error("expected default IdentityTransformer")
	}
}

// TestChannelRead_WithOptions verifies option application.
func TestChannelRead_WithOptions(t *testing.T) {
	reg := channels.NewRegistry()
	cr := NewChannelRead(reg,
		WithSelector(NewSpecificChannelsSelector("a")),
		WithTransformer(NewMappingTransformer(map[string]string{"a": "b"})),
	)
	if cr == nil {
		t.Fatal("expected non-nil")
	}
}

// TestChannelRead_Read verifies reading from registered channels.
func TestChannelRead_Read(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("greeting")
	ch.Update([]interface{}{"hello"})
	reg.Register("greeting", ch)

	cr := NewChannelRead(reg)
	values, err := cr.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if values["greeting"] != "hello" {
		t.Errorf("expected 'hello', got %v", values["greeting"])
	}
}

// TestChannelRead_EmptyRegistry verifies reading empty registry.
func TestChannelRead_EmptyRegistry(t *testing.T) {
	reg := channels.NewRegistry()
	cr := NewChannelRead(reg)
	values, err := cr.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(values) != 0 {
		t.Errorf("expected empty map, got %d entries", len(values))
	}
}

// ---- Selectors ----

// TestAllChannelsSelector verifies selecting all registered channels.
func TestAllChannelsSelector(t *testing.T) {
	reg := channels.NewRegistry()
	ch1 := channels.NewLastValue("")
	ch1.SetKey("a")
	ch1.Update([]interface{}{1})
	ch2 := channels.NewLastValue("")
	ch2.SetKey("b")
	ch2.Update([]interface{}{2})

	reg.Register("a", ch1)
	reg.Register("b", ch2)

	sel := &AllChannelsSelector{}
	names, err := sel.Select(reg)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(names) < 2 {
		t.Errorf("expected at least 2 channel names, got %d", len(names))
	}
}

// TestSpecificChannelsSelector verifies selecting specific channels.
func TestSpecificChannelsSelector(t *testing.T) {
	reg := channels.NewRegistry()
	ch1 := channels.NewLastValue("")
	ch1.SetKey("target")
	ch1.Update([]interface{}{"data"})
	ch2 := channels.NewLastValue("")
	ch2.SetKey("other")

	reg.Register("target", ch1)
	reg.Register("other", ch2)

	sel := NewSpecificChannelsSelector("target")
	names, err := sel.Select(reg)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(names) != 1 || names[0] != "target" {
		t.Errorf("expected ['target'], got %v", names)
	}
}

// TestAvailableChannelsSelector verifies selecting available channels.
func TestAvailableChannelsSelector(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("active")
	ch.Update([]interface{}{"data"})

	reg.Register("active", ch)

	sel := &AvailableChannelsSelector{}
	names, err := sel.Select(reg)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(names) == 0 {
		t.Error("expected at least 1 available channel")
	}
}

// TestPrefixChannelsSelector verifies prefix-based selection.
func TestPrefixChannelsSelector(t *testing.T) {
	reg := channels.NewRegistry()
	ch := channels.NewLastValue("")
	ch.SetKey("pre_alpha")
	ch.Update([]interface{}{1})

	reg.Register("pre_alpha", ch)

	sel := NewPrefixChannelsSelector("pre_")
	names, err := sel.Select(reg)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(names) == 0 {
		t.Error("expected at least 1 prefixed channel")
	}
}

// ---- Transformers ----

// TestIdentityTransformer verifies passthrough behavior.
func TestIdentityTransformer(t *testing.T) {
	tf := &IdentityTransformer{}
	input := map[string]interface{}{"key": "value"}
	result, err := tf.Transform(input)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
}

// TestMappingTransformer verifies key mapping.
func TestMappingTransformer(t *testing.T) {
	tf := NewMappingTransformer(map[string]string{"old": "new"})
	input := map[string]interface{}{"old": "data"}
	result, err := tf.Transform(input)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if result["new"] != "data" {
		t.Errorf("expected 'data' under key 'new', got key=%v", result)
	}
}

// TestFilterTransformer verifies filtering.
func TestFilterTransformer(t *testing.T) {
	tf := NewFilterTransformer("keep")
	input := map[string]interface{}{"keep": "val", "remove": "ignored"}
	result, err := tf.Transform(input)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if _, ok := result["remove"]; ok {
		t.Error("expected 'remove' to be filtered out")
	}
	if result["keep"] != "val" {
		t.Errorf("expected 'val', got %v", result["keep"])
	}
}

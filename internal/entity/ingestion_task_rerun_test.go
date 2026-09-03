package entity

import "testing"

func TestIngestionTaskRerunSchemaRoundTrip(t *testing.T) {
	dsl := JSONMap{"components": map[string]interface{}{"c1": map[string]interface{}{}}}
	schema := NewIngestionTaskRerunSchema(dsl, "log-1", "c1")

	task := &IngestionTask{Schema: schema}
	info, ok := task.RerunInfo()
	if !ok {
		t.Fatal("RerunInfo = false, want true")
	}
	if info.LogID != "log-1" || info.ComponentID != "c1" {
		t.Fatalf("info = %+v", info)
	}
	if _, ok := info.DSL["components"]; !ok {
		t.Fatalf("dsl = %v", info.DSL)
	}
}

func TestIngestionTaskRerunInfoAbsent(t *testing.T) {
	task := &IngestionTask{Schema: JSONMap{"other": true}}
	if _, ok := task.RerunInfo(); ok {
		t.Fatal("RerunInfo = true, want false")
	}
}

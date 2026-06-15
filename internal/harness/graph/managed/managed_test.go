// Package managed tests managed value functionality.
package managed

import (
	"testing"
)

func TestIsManagedValue(t *testing.T) {
	isLastStep := NewIsLastStep()

	if !IsManagedValue(isLastStep) {
		t.Error("IsLastStep should be a managed value")
	}

	if IsManagedValue("not a managed value") {
		t.Error("String should not be a managed value")
	}

	if IsManagedValue(nil) {
		t.Error("Nil should not be a managed value")
	}
}

func TestManagedValueSpec(t *testing.T) {
	spec := NewManagedValueSpec(
		"TestValue",
		func() ManagedValue { return NewIsLastStep() },
		"default",
	)

	if spec.Name != "TestValue" {
		t.Errorf("Expected name 'TestValue', got '%s'", spec.Name)
	}

	if spec.Default != "default" {
		t.Errorf("Expected default 'default', got '%v'", spec.Default)
	}

	if spec.Factory == nil {
		t.Error("Factory should not be nil")
	}
}

func TestIsManagedValueSpec(t *testing.T) {
	spec := NewManagedValueSpec(
		"TestValue",
		func() ManagedValue { return NewIsLastStep() },
		nil,
	)

	if !IsManagedValueSpec(spec) {
		t.Error("ManagedValueSpec should be recognized")
	}

	if IsManagedValueSpec("not a spec") {
		t.Error("String should not be a managed value spec")
	}
}

func TestIsValueManaged(t *testing.T) {
	isLastStep := NewIsLastStep()
	spec := NewManagedValueSpec("test", func() ManagedValue { return NewIsLastStep() }, nil)

	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"IsLastStep", isLastStep, true},
		{"Spec", spec, true},
		{"String", "test", false},
		{"Nil", nil, false},
		{"Number", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := IsValueManaged(tt.value); result != tt.expected {
				t.Errorf("IsValueManaged(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestGetManagedValueName(t *testing.T) {
	isLastStep := NewIsLastStep()
	spec := NewManagedValueSpec("CustomSpec", func() ManagedValue { return NewIsLastStep() }, nil)

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"IsLastStep", isLastStep, "IsLastStep"},
		{"Spec", spec, "CustomSpec"},
		{"Nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := GetManagedValueName(tt.value); result != tt.expected {
				t.Errorf("GetManagedValueName(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestCurrentStep(t *testing.T) {
	currentStep := NewCurrentStep()

	// Test default value
	if currentStep.Value != 0 {
		t.Errorf("Expected initial value 0, got %d", currentStep.Value)
	}

	// Test Name
	if currentStep.Name() != "CurrentStep" {
		t.Errorf("Expected name 'CurrentStep', got '%s'", currentStep.Name())
	}

	// Test Set
	currentStep.Set(5)
	if currentStep.Value != 5 {
		t.Errorf("Expected value 5 after Set, got %d", currentStep.Value)
	}

	// Test Increment
	currentStep.Increment()
	if currentStep.Value != 6 {
		t.Errorf("Expected value 6 after Increment, got %d", currentStep.Value)
	}

	// Test Copy
	copied := currentStep.Copy().(*CurrentStep)
	if copied.Value != 6 {
		t.Errorf("Expected copied value 6, got %d", copied.Value)
	}

	// Verify they are independent
	currentStep.Value = 10
	if copied.Value != 6 {
		t.Errorf("Copied value should remain 6, got %d", copied.Value)
	}
}

func TestConfigValue(t *testing.T) {
	configValue := NewConfigValue("api_key", "default_key")

	// Test with empty scratchpad
	val, err := configValue.Get(nil)
	if err != nil {
		t.Errorf("Get should not error on empty scratchpad: %v", err)
	}
	if val != "default_key" {
		t.Errorf("Expected default value 'default_key', got '%v'", val)
	}

	// Test with scratchpad containing configurable
	scratchpad := map[string]interface{}{
		"configurable": map[string]interface{}{
			"api_key": "custom_key",
		},
	}
	val, err = configValue.Get(scratchpad)
	if err != nil {
		t.Errorf("Get should not error: %v", err)
	}
	if val != "custom_key" {
		t.Errorf("Expected value 'custom_key', got '%v'", val)
	}

	// Test Name
	name := configValue.Name()
	expectedName := "ConfigValue[api_key]"
	if name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, name)
	}

	// Test Copy
	copied := configValue.Copy().(*ConfigValue)
	if copied.Key != "api_key" {
		t.Errorf("Expected copied key 'api_key', got '%s'", copied.Key)
	}
}

func TestTaskID(t *testing.T) {
	taskID := NewTaskID()

	// Test default value
	if taskID.Value != "" {
		t.Errorf("Expected empty string, got '%s'", taskID.Value)
	}

	// Test with scratchpad containing task_id
	scratchpad := map[string]interface{}{
		"task_id": "task-123",
	}
	val, err := taskID.Get(scratchpad)
	if err != nil {
		t.Errorf("Get should not error: %v", err)
	}
	if val != "task-123" {
		t.Errorf("Expected value 'task-123', got '%v'", val)
	}

	// Test Name
	if taskID.Name() != "TaskID" {
		t.Errorf("Expected name 'TaskID', got '%s'", taskID.Name())
	}
}

func TestNodeName(t *testing.T) {
	nodeName := NewNodeName()

	// Test default value
	if nodeName.Value != "" {
		t.Errorf("Expected empty string, got '%s'", nodeName.Value)
	}

	// Test with scratchpad containing node_name
	scratchpad := map[string]interface{}{
		"node_name": "agent_node",
	}
	val, err := nodeName.Get(scratchpad)
	if err != nil {
		t.Errorf("Get should not error: %v", err)
	}
	if val != "agent_node" {
		t.Errorf("Expected value 'agent_node', got '%v'", val)
	}

	// Test Name
	if nodeName.Name() != "NodeName" {
		t.Errorf("Expected name 'NodeName', got '%s'", nodeName.Name())
	}
}

func TestManagedValueGetValue(t *testing.T) {
	spec := NewManagedValueSpec(
		"TestValue",
		func() ManagedValue {
			mv := NewCurrentStep()
			mv.Value = 42
			return mv
		},
		"default",
	)

	// Test with empty scratchpad - should return default
	val := spec.GetValue(nil)
	if val != "default" {
		t.Errorf("Expected default value 'default', got '%v'", val)
	}

	// Test Create
	mv := spec.Create()
	if mv == nil {
		t.Error("Create should return a managed value")
	}

	currentStep, ok := mv.(*CurrentStep)
	if !ok {
		t.Error("Create should return a CurrentStep")
	}
	if currentStep.Value != 42 {
		t.Errorf("Expected value 42, got %d", currentStep.Value)
	}
}

type TestStruct struct {
	ManagedField   ManagedValue
	SpecField      *ManagedValueSpec
	NormalField    string
	NilField       interface{}
}

func TestExtractManagedValues(t *testing.T) {
	testStruct := TestStruct{
		ManagedField: NewIsLastStep(),
		SpecField:    NewManagedValueSpec("test", func() ManagedValue { return NewCurrentStep() }, nil),
		NormalField:  "normal",
		NilField:     nil,
	}

	values := ExtractManagedValues(&testStruct)

	if len(values) != 1 {
		t.Errorf("Expected 1 managed value, got %d", len(values))
	}

	if values[0].Name() != "IsLastStep" {
		t.Errorf("Expected IsLastStep, got %s", values[0].Name())
	}
}

func TestExtractManagedValueSpecs(t *testing.T) {
	testStruct := TestStruct{
		ManagedField: NewIsLastStep(),
		SpecField:    NewManagedValueSpec("test", func() ManagedValue { return NewCurrentStep() }, nil),
		NormalField:  "normal",
		NilField:     nil,
	}

	specs := ExtractManagedValueSpecs(&testStruct)

	if len(specs) != 1 {
		t.Errorf("Expected 1 managed value spec, got %d", len(specs))
	}

	if specs[0].Name != "test" {
		t.Errorf("Expected name 'test', got %s", specs[0].Name)
	}
}

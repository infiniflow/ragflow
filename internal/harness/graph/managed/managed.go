// Package managed provides managed value types for runtime injection.
package managed

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

// ManagedValue represents a value that is managed by the runtime.
type ManagedValue interface {
	// Get returns the current value of the managed value.
	Get(scratchpad interface{}) (interface{}, error)
	// Copy creates a copy of this managed value.
	Copy() ManagedValue
	// Name returns the name of this managed value.
	Name() string
}

// ManagedValueMapping is a concurrency-safe collection of managed values keyed by name.
type ManagedValueMapping struct {
	mu   sync.RWMutex
	data map[string]ManagedValue
}

// NewManagedValueMapping creates a new managed value mapping.
func NewManagedValueMapping() *ManagedValueMapping {
	return &ManagedValueMapping{
		data: make(map[string]ManagedValue),
	}
}

// Register registers a managed value.
func (m *ManagedValueMapping) Register(name string, value ManagedValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = value
}

// Get gets a managed value by name.
func (m *ManagedValueMapping) Get(name string) (ManagedValue, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[name]
	return val, ok
}

// Contains checks if a managed value exists.
func (m *ManagedValueMapping) Contains(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[name]
	return ok
}

// Names returns all managed value names.
func (m *ManagedValueMapping) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.data))
	for name := range m.data {
		names = append(names, name)
	}
	return names
}

// IsLastStep provides information about whether the current step is the last step.
type IsLastStep struct {
	// Value indicates if this is the last step.
	Value bool
}

// NewIsLastStep creates a new IsLastStep managed value.
func NewIsLastStep() *IsLastStep {
	return &IsLastStep{
		Value: false,
	}
}

// Get returns the current value.
func (v *IsLastStep) Get(scratchpad interface{}) (interface{}, error) {
	if sd, ok := scratchpad.(map[string]interface{}); ok {
		if val, exists := sd["is_last_step"]; exists {
			if bl, ok := val.(bool); ok {
				v.Value = bl
			}
		}
	}
	return v.Value, nil
}

// Set sets the value.
func (v *IsLastStep) Set(value bool) {
	v.Value = value
}

// Name returns the name of this managed value.
func (v *IsLastStep) Name() string {
	return "IsLastStep"
}

// Copy creates a copy of this managed value.
func (v *IsLastStep) Copy() ManagedValue {
	return &IsLastStep{
		Value: v.Value,
	}
}

// IsManagedValue checks if a value is a managed value.
func IsManagedValue(val interface{}) bool {
	_, ok := val.(ManagedValue)
	return ok
}

// CurrentStep provides information about the current step number.
type CurrentStep struct {
	// Value is the current step number.
	Value int
}

// NewCurrentStep creates a new CurrentStep managed value.
func NewCurrentStep() *CurrentStep {
	return &CurrentStep{
		Value: 0,
	}
}

// Get returns the current step number.
func (v *CurrentStep) Get(scratchpad interface{}) (interface{}, error) {
	if sd, ok := scratchpad.(map[string]interface{}); ok {
		if val, exists := sd["current_step"]; exists {
			if num, ok := val.(int); ok {
				v.Value = num
			}
		}
	}
	return v.Value, nil
}

// Set sets the step number.
func (v *CurrentStep) Set(value int) {
	v.Value = value
}

// Increment increments the step number.
func (v *CurrentStep) Increment() {
	v.Value++
}

// Name returns the name of this managed value.
func (v *CurrentStep) Name() string {
	return "CurrentStep"
}

// Copy creates a copy of this managed value.
func (v *CurrentStep) Copy() ManagedValue {
	return &CurrentStep{
		Value: v.Value,
	}
}

// ConfigValue provides access to configurable values.
type ConfigValue struct {
	// Key is the configuration key.
	Key string
	// Default is the default value if not found.
	Default interface{}
}

// NewConfigValue creates a new ConfigValue managed value.
func NewConfigValue(key string, defaultValue interface{}) *ConfigValue {
	return &ConfigValue{
		Key:     key,
		Default: defaultValue,
	}
}

// Get returns the configuration value.
func (v *ConfigValue) Get(scratchpad interface{}) (interface{}, error) {
	if sd, ok := scratchpad.(map[string]interface{}); ok {
		if configurable, ok := sd["configurable"].(map[string]interface{}); ok {
			if val, exists := configurable[v.Key]; exists {
				return val, nil
			}
		}
	}
	return v.Default, nil
}

// Name returns the name of this managed value.
func (v *ConfigValue) Name() string {
	return fmt.Sprintf("ConfigValue[%s]", v.Key)
}

// Copy creates a copy of this managed value.
func (v *ConfigValue) Copy() ManagedValue {
	return &ConfigValue{
		Key:     v.Key,
		Default: v.Default,
	}
}

// TaskID provides access to the current task ID.
type TaskID struct {
	// Value is the task ID.
	Value string
}

// NewTaskID creates a new TaskID managed value.
func NewTaskID() *TaskID {
	return &TaskID{
		Value: "",
	}
}

// Get returns the task ID.
func (v *TaskID) Get(scratchpad interface{}) (interface{}, error) {
	if sd, ok := scratchpad.(map[string]interface{}); ok {
		if val, exists := sd["task_id"]; exists {
			if str, ok := val.(string); ok {
				v.Value = str
			}
		}
	}
	return v.Value, nil
}

// Name returns the name of this managed value.
func (v *TaskID) Name() string {
	return "TaskID"
}

// Copy creates a copy of this managed value.
func (v *TaskID) Copy() ManagedValue {
	return &TaskID{
		Value: v.Value,
	}
}

// NodeName provides access to the current node name.
type NodeName struct {
	// Value is the node name.
	Value string
}

// NewNodeName creates a new NodeName managed value.
func NewNodeName() *NodeName {
	return &NodeName{
		Value: "",
	}
}

// Get returns the node name.
func (v *NodeName) Get(scratchpad interface{}) (interface{}, error) {
	if sd, ok := scratchpad.(map[string]interface{}); ok {
		if val, exists := sd["node_name"]; exists {
			if str, ok := val.(string); ok {
				v.Value = str
			}
		}
	}
	return v.Value, nil
}

// Name returns the name of this managed value.
func (v *NodeName) Name() string {
	return "NodeName"
}

// Copy creates a copy of this managed value.
func (v *NodeName) Copy() ManagedValue {
	return &NodeName{
		Value: v.Value,
	}
}

// ManagedValueSpec specifies a managed value.
type ManagedValueSpec struct {
	// Name is the name of the managed value.
	Name string
	// Factory creates the managed value.
	Factory func() ManagedValue
	// Default is the default value if not managed.
	Default interface{}
}

// NewManagedValueSpec creates a new managed value spec.
func NewManagedValueSpec(name string, factory func() ManagedValue, defaultValue interface{}) *ManagedValueSpec {
	return &ManagedValueSpec{
		Name:    name,
		Factory: factory,
		Default: defaultValue,
	}
}

// Create creates the managed value.
func (s *ManagedValueSpec) Create() ManagedValue {
	if s.Factory != nil {
		return s.Factory()
	}
	return nil
}

// GetValue gets the value from scratchpad or returns default.
func (s *ManagedValueSpec) GetValue(scratchpad interface{}) interface{} {
	// Only try to get value from scratchpad if it's not nil/empty
	if scratchpad != nil {
		if s.Factory != nil {
			mv := s.Factory()
			if val, err := mv.Get(scratchpad); err == nil {
				return val
			}
		}
	}
	return s.Default
}

// IsValueManaged checks if a value is managed based on its type.
func IsValueManaged(val interface{}) bool {
	if val == nil {
		return false
	}
	return IsManagedValue(val) || IsManagedValueSpec(val)
}

// IsManagedValueSpec checks if a value is a managed value spec.
func IsManagedValueSpec(val interface{}) bool {
	_, ok := val.(*ManagedValueSpec)
	return ok
}

// GetManagedValueName returns the name of a managed value or spec.
func GetManagedValueName(val interface{}) string {
	if mv, ok := val.(ManagedValue); ok {
		return mv.Name()
	}
	if spec, ok := val.(*ManagedValueSpec); ok {
		return spec.Name
	}
	return ""
}

// ExtractManagedValues extracts managed values from a struct.
func ExtractManagedValues(obj interface{}) []ManagedValue {
	result := []ManagedValue{}
	
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	
	if v.Kind() != reflect.Struct {
		return result
	}
	
	for i := 0; i < v.NumField(); i++ {
		if !v.Type().Field(i).IsExported() {
			continue
		}
		field := v.Field(i)
		if IsManagedValue(field.Interface()) {
			if mv, ok := field.Interface().(ManagedValue); ok {
				result = append(result, mv)
			}
		}
	}
	
	return result
}

// ExtractManagedValueSpecs extracts managed value specs from a struct.
func ExtractManagedValueSpecs(obj interface{}) []*ManagedValueSpec {
	result := []*ManagedValueSpec{}
	
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	
	if v.Kind() != reflect.Struct {
		return result
	}
	
	for i := 0; i < v.NumField(); i++ {
		if !v.Type().Field(i).IsExported() {
			continue
		}
		field := v.Field(i)
		if IsManagedValueSpec(field.Interface()) {
			if spec, ok := field.Interface().(*ManagedValueSpec); ok {
				result = append(result, spec)
			}
		}
	}
	
	return result
}

// PregelScratchpad provides temporary storage for graph execution.
type PregelScratchpad map[string]interface{}

// NewPregelScratchpad creates a new scratchpad.
func NewPregelScratchpad() PregelScratchpad {
	return make(PregelScratchpad)
}

// GetCallCounter returns the call counter.
func (p PregelScratchpad) GetCallCounter() int {
	if val, ok := p["call_counter"].(int); ok {
		return val
	}
	return 0
}

// IncrementCallCounter increments the call counter.
func (p PregelScratchpad) IncrementCallCounter() {
	p["call_counter"] = p.GetCallCounter() + 1
}

// SetCallCounter sets the call counter.
func (p PregelScratchpad) SetCallCounter(value int) {
	p["call_counter"] = value
}

// GetInterruptCounter returns the interrupt counter.
func (p PregelScratchpad) GetInterruptCounter() int {
	if val, ok := p["interrupt_counter"].(int); ok {
		return val
	}
	return 0
}

// IncrementInterruptCounter increments the interrupt counter.
func (p PregelScratchpad) IncrementInterruptCounter() {
	p["interrupt_counter"] = p.GetInterruptCounter() + 1
}

// GetSubgraphCounter returns the subgraph counter.
func (p PregelScratchpad) GetSubgraphCounter() int {
	if val, ok := p["subgraph_counter"].(int); ok {
		return val
	}
	return 0
}

// IncrementSubgraphCounter increments the subgraph counter.
func (p PregelScratchpad) IncrementSubgraphCounter() {
	p["subgraph_counter"] = p.GetSubgraphCounter() + 1
}

// Get returns a value from the scratchpad.
func (p PregelScratchpad) Get(key string) (interface{}, bool) {
	val, ok := p[key]
	return val, ok
}

// Set sets a value in the scratchpad.
func (p PregelScratchpad) Set(key string, value interface{}) {
	p[key] = value
}

// Delete removes a value from the scratchpad.
func (p PregelScratchpad) Delete(key string) {
	delete(p, key)
}

// Clear removes all values from the scratchpad.
func (p PregelScratchpad) Clear() {
	for k := range p {
		delete(p, k)
	}
}

// Clone creates a copy of the scratchpad.
func (p PregelScratchpad) Clone() PregelScratchpad {
	clone := make(PregelScratchpad, len(p))
	for k, v := range p {
		clone[k] = v
	}
	return clone
}

// ConfigKey represents keys used in runtime configuration.
const (
	ManagedConfigKeyTaskID      = "__task_id__"
	ManagedConfigKeyRuntime     = "__runtime__"
	ManagedConfigKeyRead        = "__read__"
	ManagedConfigKeySend        = "__send__"
	ManagedConfigKeyWriter      = "__writer__"
	ManagedConfigKeyStore       = "__store__"
	ManagedConfigKeyPrevious    = "__previous__"
	ManagedConfigKeyCheckpointNS = "__checkpoint_ns__"
	ManagedConfigKeyConfigurable = "__configurable__"
)

// Runtime provides runtime information for graph execution.
// This corresponds to Python's Runtime class in runtime.py
type Runtime struct {
	// TaskID is the ID of the current task.
	TaskID string
	// NodeName is the name of the current node.
	NodeName string
	// Step is the current step number.
	Step int
	// Configurable is the configurable parameters.
	Configurable map[string]interface{}
	// CheckpointNS is the checkpoint namespace.
	CheckpointNS string
	// Context is the static context for the graph run, like user_id, db_conn, etc.
	// This is used for multi-tenant support.
	Context interface{}
	// Store is the BaseStore for long-term storage, enabling persistence and memory.
	Store interface{}
	// StreamWriter is the function that writes to the custom stream.
	StreamWriter func(interface{})
	// Previous is the previous return value for the given thread.
	Previous interface{}
}

// NewRuntime creates a new runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		TaskID:       "",
		NodeName:     "",
		Step:         0,
		Configurable: make(map[string]interface{}),
		CheckpointNS: "",
		Context:      nil,
		Store:        nil,
		StreamWriter: nil,
		Previous:     nil,
	}
}

// Clone creates a copy of the runtime.
func (r *Runtime) Clone() *Runtime {
	return &Runtime{
		TaskID:       r.TaskID,
		NodeName:     r.NodeName,
		Step:         r.Step,
		Configurable: cloneMap(r.Configurable),
		CheckpointNS: r.CheckpointNS,
		Context:      r.Context,
		Store:        r.Store,
		StreamWriter: r.StreamWriter,
		Previous:     r.Previous,
	}
}

// Merge merges two runtimes together.
// If a value is not provided in the other runtime, the value from the current runtime is used.
func (r *Runtime) Merge(other *Runtime) *Runtime {
	if other == nil {
		return r.Clone()
	}

	merged := r.Clone()

	if other.Context != nil {
		merged.Context = other.Context
	}
	if other.Store != nil {
		merged.Store = other.Store
	}
	if other.StreamWriter != nil {
		merged.StreamWriter = other.StreamWriter
	}
	if other.Previous != nil {
		merged.Previous = other.Previous
	}
	if other.TaskID != "" {
		merged.TaskID = other.TaskID
	}
	if other.NodeName != "" {
		merged.NodeName = other.NodeName
	}
	if other.Step != 0 {
		merged.Step = other.Step
	}
	if other.CheckpointNS != "" {
		merged.CheckpointNS = other.CheckpointNS
	}

	// Merge configurable
	for k, v := range other.Configurable {
		merged.Configurable[k] = v
	}

	return merged
}

// Set sets a value in the runtime's Configurable map.
func (r *Runtime) Set(ctx context.Context, key string, value interface{}) {
	if r.Configurable == nil {
		r.Configurable = make(map[string]interface{})
	}
	r.Configurable[key] = value
}

// Get gets a value from the runtime's Configurable map.
func (r *Runtime) Get(ctx context.Context, key string) (interface{}, bool) {
	if r.Configurable == nil {
		return nil, false
	}
	val, ok := r.Configurable[key]
	return val, ok
}

// Override creates a new runtime with the given overrides.
func (r *Runtime) Override(overrides map[string]interface{}) *Runtime {
	newRuntime := r.Clone()

	if context, ok := overrides["context"]; ok {
		newRuntime.Context = context
	}
	if store, ok := overrides["store"]; ok {
		newRuntime.Store = store
	}
	if streamWriter, ok := overrides["stream_writer"]; ok {
		if sw, ok := streamWriter.(func(interface{})); ok {
			newRuntime.StreamWriter = sw
		}
	}
	if previous, ok := overrides["previous"]; ok {
		newRuntime.Previous = previous
	}
	if taskID, ok := overrides["task_id"]; ok {
		if tid, ok := taskID.(string); ok {
			newRuntime.TaskID = tid
		}
	}
	if nodeName, ok := overrides["node_name"]; ok {
		if nn, ok := nodeName.(string); ok {
			newRuntime.NodeName = nn
		}
	}
	if step, ok := overrides["step"]; ok {
		if s, ok := step.(int); ok {
			newRuntime.Step = s
		}
	}
	if checkpointNS, ok := overrides["checkpoint_ns"]; ok {
		if ns, ok := checkpointNS.(string); ok {
			newRuntime.CheckpointNS = ns
		}
	}

	return newRuntime
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// DEFAULT_RUNTIME is the default runtime instance with nil values.
// Configurable is nil (not an empty map) so that direct mutation via Set
// panics with nil pointer dereference rather than silently corrupting a
// shared global. Callers must use Clone() to obtain a safe copy.
// This corresponds to Python's DEFAULT_RUNTIME in runtime.py
var DEFAULT_RUNTIME = &Runtime{
	TaskID:       "",
	NodeName:     "",
	Step:         0,
	Configurable: nil,
	CheckpointNS: "",
	Context:      nil,
	Store:        nil,
	StreamWriter: nil,
	Previous:     nil,
}

// get_runtime returns the runtime for the current graph run.
// This corresponds to Python's get_runtime() function in runtime.py
func get_runtime(config map[string]interface{}) *Runtime {
	if config == nil {
		return DEFAULT_RUNTIME.Clone()
	}
	if runtime, ok := config[ManagedConfigKeyRuntime].(*Runtime); ok {
		return runtime
	}
	return DEFAULT_RUNTIME.Clone()
}

// GetTaskID returns the task ID from config.
func GetTaskID(config map[string]interface{}) string {
	if config == nil {
		return ""
	}
	if val, ok := config[ManagedConfigKeyTaskID].(string); ok {
		return val
	}
	return ""
}

// GetRuntime returns the runtime from config.
func GetRuntime(config map[string]interface{}) *Runtime {
	if config == nil {
		return NewRuntime()
	}
	if val, ok := config[ManagedConfigKeyRuntime].(*Runtime); ok {
		return val
	}
	return NewRuntime()
}

// SetRuntime sets the runtime in config.
func SetRuntime(config map[string]interface{}, runtime *Runtime) {
	if config == nil {
		return
	}
	config[ManagedConfigKeyRuntime] = runtime
}

// GetReader returns the read function from config.
func GetReader(config map[string]interface{}) interface{} {
	if config == nil {
		return nil
	}
	return config[ManagedConfigKeyRead]
}

// SetReader sets the read function in config.
func SetReader(config map[string]interface{}, reader interface{}) {
	if config == nil {
		return
	}
	config[ManagedConfigKeyRead] = reader
}

// GetSend returns the send function from config.
func GetSend(config map[string]interface{}) func(...interface{}) {
	if config == nil {
		return nil
	}
	if val, ok := config[ManagedConfigKeySend]; ok {
		if fn, ok := val.(func(...interface{})); ok {
			return fn
		}
	}
	return nil
}

// SetSend sets the send function in config.
func SetSend(config map[string]interface{}, send func(...interface{})) {
	if config == nil {
		return
	}
	config[ManagedConfigKeySend] = send
}

// GetWriter returns the writer from config.
func GetWriter(config map[string]interface{}) interface{} {
	if config == nil {
		return nil
	}
	return config[ManagedConfigKeyWriter]
}

// SetWriter sets the writer in config.
func SetWriter(config map[string]interface{}, writer interface{}) {
	if config == nil {
		return
	}
	config[ManagedConfigKeyWriter] = writer
}





// PatchConfig patches a config with new values.
func PatchConfig(base map[string]interface{}, updates map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = make(map[string]interface{})
	}
	if updates == nil {
		return base
	}
	
	result := make(map[string]interface{}, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
}

// PatchConfigurable patches the configurable section of a config.
func PatchConfigurable(base map[string]interface{}, updates map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = make(map[string]interface{})
	}
	if updates == nil {
		return base
	}
	
	// Get or create configurable section — deep copy to avoid mutating the original.
	configurable := make(map[string]interface{})
	if cfg, ok := base[ManagedConfigKeyConfigurable]; ok {
		if cfgMap, ok := cfg.(map[string]interface{}); ok {
			for k, v := range cfgMap {
				configurable[k] = v
			}
		}
	}
	
	// Merge updates
	for k, v := range updates {
		configurable[k] = v
	}
	
	// Update base config
	result := make(map[string]interface{}, len(base)+1)
	for k, v := range base {
		result[k] = v
	}
	result[ManagedConfigKeyConfigurable] = configurable
	
	return result
}

// GetConfigurable returns the configurable section from config.
func GetConfigurable(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	if val, ok := config[ManagedConfigKeyConfigurable]; ok {
		if cfgMap, ok := val.(map[string]interface{}); ok {
			return cfgMap
		}
	}
	return nil
}

// GetCheckpointNS returns the checkpoint namespace from config.
func GetCheckpointNS(config map[string]interface{}) string {
	if config == nil {
		return ""
	}
	if val, ok := config[ManagedConfigKeyCheckpointNS].(string); ok {
		return val
	}
	return ""
}

// SetCheckpointNS sets the checkpoint namespace in config.
func SetCheckpointNS(config map[string]interface{}, ns string) {
	if config == nil {
		return
	}
	config[ManagedConfigKeyCheckpointNS] = ns
}

// ParseCheckpointNS parses a checkpoint namespace to extract node path.
func ParseCheckpointNS(ns string) []string {
	if ns == "" {
		return []string{}
	}
	return splitCheckpointNS(ns)
}

// RecastCheckpointNS recasts a checkpoint namespace by removing task ID.
func RecastCheckpointNS(ns string) string {
	parts := splitCheckpointNS(ns)
	if len(parts) == 0 {
		return ""
	}
	
	// Remove task ID if present (usually the last part)
	lastPart := parts[len(parts)-1]
	if isTaskID(lastPart) {
		return joinCheckpointNS(parts[:len(parts)-1])
	}
	
	return ns
}

func splitCheckpointNS(ns string) []string {
	return strings.Split(ns, "|")
}

func joinCheckpointNS(parts []string) string {
	// Simple implementation - join with separator
	// In a full implementation, this would use proper namespace separator
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "|" + parts[i]
	}
	return result
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isTaskID(s string) bool {
	return uuidRE.MatchString(s)
}

// StreamWriter is a function that writes to the output stream.
type StreamWriter func(interface{})

// NewStreamWriter creates a new stream writer.
func NewStreamWriter(fn func(interface{})) StreamWriter {
	return StreamWriter(fn)
}

// Write writes a value to the stream.
func (w StreamWriter) Write(value interface{}) {
	w(value)
}

// FormatCheckpoint formats a checkpoint for debug output.
func FormatCheckpoint(checkpoint map[string]interface{}) string {
	if checkpoint == nil {
		return "{}"
	}
	
	result := "{"
	first := true
	for k, v := range checkpoint {
		if !first {
			result += ", "
		}
		result += fmt.Sprintf("\"%s\": %v", k, v)
		first = false
	}
	result += "}"
	return result
}

// FormatTask formats a task for debug output.
func FormatTask(task interface{}) string {
	if task == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", task)
}

// FormatValue formats a value for debug output.
func FormatValue(value interface{}) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%v", value)
}

// FormatDuration formats a duration for debug output.
func FormatDuration(d int64) string {
	if d < 1000 {
		return fmt.Sprintf("%dms", d)
	} else if d < 60000 {
		return fmt.Sprintf("%.1fs", float64(d)/1000)
	} else if d < 3600000 {
		return fmt.Sprintf("%.1fm", float64(d)/60000)
	} else {
		return fmt.Sprintf("%.1fh", float64(d)/3600000)
	}
}

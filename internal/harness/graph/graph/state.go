// Package graph provides graph building capabilities for LangGraph Go.
package graph

import (
	"fmt"
	"reflect"
	"strings"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/types"
)

// Annotation holds metadata for a state field.
type Annotation struct {
	// Reducer specifies a custom reducer function for this field.
	Reducer types.ReducerFunc
	// Optional metadata for documentation or tooling.
	Metadata map[string]interface{}
}

// fieldInfo holds processed information about a state field.
type fieldInfo struct {
	Name       string
	Type       reflect.Type
	Channel    channels.Channel
	Annotation *Annotation
}

// validateStateSchema validates the state schema.
// It returns a map of field names to fieldInfo, or an error.
func validateStateSchema(schema interface{}) (map[string]*fieldInfo, error) {
	if schema == nil {
		return nil, fmt.Errorf("state schema cannot be nil")
	}

	v := reflect.ValueOf(schema)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fieldInfos := make(map[string]*fieldInfo)

	switch v.Kind() {
	case reflect.Struct:
		// Process struct fields
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				// Unexported field
				continue
			}

			info, err := processField(field)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
			fieldInfos[info.Name] = info
		}

	case reflect.Map:
		// For map types, we expect map[string]interface{} or similar.
		// We'll validate that keys are strings.
		t := v.Type()
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("state schema map must have string keys")
		}
		// For maps, we can't extract field annotations statically.
		// We'll treat each potential key as a field with default channel.
		// In practice, channels are added dynamically via AddChannel.
		// So we just accept the map type.

	default:
		return nil, fmt.Errorf("state schema must be a struct or map, got %v", v.Kind())
	}

	return fieldInfos, nil
}

// processField extracts field information and annotations.
func processField(field reflect.StructField) (*fieldInfo, error) {
	info := &fieldInfo{
		Name: field.Name,
		Type: field.Type,
	}

	// Parse struct tags for annotations
	tag := field.Tag.Get("harness")
	if tag != "" {
		annotation, err := parseAnnotation(tag)
		if err != nil {
			return nil, fmt.Errorf("invalid annotation: %w", err)
		}
		info.Annotation = annotation
	}

	// Determine reducer function
	var reducer types.ReducerFunc
	if info.Annotation != nil && info.Annotation.Reducer != nil {
		reducer = info.Annotation.Reducer
	}

	// Create appropriate channel with reducer
	channel, err := channels.CreateReducerChannel(field.Name, field.Type, reducer)
	if err != nil {
		return nil, fmt.Errorf("failed to create channel for field %s: %w", field.Name, err)
	}
	info.Channel = channel

	return info, nil
}

// parseAnnotation parses a harness struct tag into an Annotation.
// Format: "reducer=add" or "reducer=custom,meta=value"
func parseAnnotation(tag string) (*Annotation, error) {
	annotation := &Annotation{
		Metadata: make(map[string]interface{}),
	}

	pairs := strings.Split(tag, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			// Could be a boolean flag
			annotation.Metadata[pair] = true
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "reducer":
			// Map reducer name to function
			reducer, ok := reducers[value]
			if !ok {
				return nil, fmt.Errorf("unknown reducer: %s", value)
			}
			annotation.Reducer = reducer
		default:
			annotation.Metadata[key] = value
		}
	}

	return annotation, nil
}

// reducers is a registry of built-in reducer functions.
var reducers = map[string]types.ReducerFunc{
	// Add reducer for numeric types
	"add": func(current, update interface{}) interface{} {
		if current == nil {
			return update
		}
		// Simple addition for ints and floats
		switch c := current.(type) {
		case int:
			if u, ok := update.(int); ok {
				return c + u
			}
		case float64:
			if u, ok := update.(float64); ok {
				return c + u
			}
		}
		// If types don't match, return update (overwrite)
		return update
	},
	// Append reducer for slices
	"append": func(current, update interface{}) interface{} {
		if current == nil {
			return []interface{}{update}
		}
		if slice, ok := current.([]interface{}); ok {
			return append(slice, update)
		}
		// If not a slice, convert to slice
		return []interface{}{current, update}
	},
	// Merge reducer for maps
	"merge": func(current, update interface{}) interface{} {
		if current == nil {
			return update
		}
		if currentMap, ok := current.(map[string]interface{}); ok {
			if updateMap, ok := update.(map[string]interface{}); ok {
				result := make(map[string]interface{}, len(currentMap)+len(updateMap))
				for k, v := range currentMap {
					result[k] = v
				}
				for k, v := range updateMap {
					result[k] = v
				}
				return result
			}
		}
		// If types don't match, return update (overwrite)
		return update
	},
}

// ValidateStateSchema validates the graph's state schema.
// This should be called during graph compilation or explicitly by users.
func (g *stateGraph) ValidateStateSchema() error {
	_, err := validateStateSchema(g.stateSchema)
	return err
}

// GetStateSchemaInfo returns processed information about the state schema.
// Useful for debugging and tooling.
func (g *stateGraph) GetStateSchemaInfo() (map[string]*fieldInfo, error) {
	return validateStateSchema(g.stateSchema)
}

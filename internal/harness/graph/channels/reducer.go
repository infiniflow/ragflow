package channels

import (
	"reflect"

	"ragflow/internal/harness/graph/types"
)

// ReducerChannel wraps a channel with a reducer function.
type ReducerChannel struct {
	Channel
	reducer types.ReducerFunc
}

// NewReducerChannel creates a new ReducerChannel.
func NewReducerChannel(channel Channel, reducer types.ReducerFunc) *ReducerChannel {
	return &ReducerChannel{
		Channel: channel,
		reducer: reducer,
	}
}

// Update applies the reducer to combine new values with the current value.
func (rc *ReducerChannel) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}

	// Read current value from the wrapped channel.
	current, err := rc.Channel.Get()

	// Combine all values with the current value using the reducer.
	// If the channel is empty, start from values[0] and combine the rest.
	combined := values[0]
	if err == nil {
		combined = rc.reducer(current, combined)
	}
	for i := 1; i < len(values); i++ {
		combined = rc.reducer(combined, values[i])
	}

	return rc.Channel.Update([]interface{}{combined})
}

// Copy returns a copy of the ReducerChannel.
func (rc *ReducerChannel) Copy() Channel {
	return &ReducerChannel{
		Channel: rc.Channel.Copy(),
		reducer: rc.reducer,
	}
}

// Checkpoint returns the checkpoint of the wrapped channel.
func (rc *ReducerChannel) Checkpoint() interface{} {
	return rc.Channel.Checkpoint()
}

// FromCheckpoint restores the wrapped channel from a checkpoint.
func (rc *ReducerChannel) FromCheckpoint(checkpoint interface{}) Channel {
	return &ReducerChannel{
		Channel: rc.Channel.FromCheckpoint(checkpoint),
		reducer: rc.reducer,
	}
}

// CreateReducerChannel creates a reducer channel based on type hints.
// It inspects the field type and annotation to determine appropriate channel.
func CreateReducerChannel(fieldName string, fieldType reflect.Type, reducer types.ReducerFunc) (Channel, error) {
	// Determine default channel based on type
	var channel Channel

	switch fieldType.Kind() {
	case reflect.Slice, reflect.Array:
		// For slices, use BinaryOperatorAggregate with append operator
		channel = NewBinaryOperatorAggregate(fieldType, ListAppend)
	case reflect.Map:
		// For maps, use BinaryOperatorAggregate with merge operator
		channel = NewBinaryOperatorAggregate(fieldType, func(a, b interface{}) interface{} {
			if aMap, ok := a.(map[string]interface{}); ok {
				if bMap, ok := b.(map[string]interface{}); ok {
					result := make(map[string]interface{}, len(aMap)+len(bMap))
					for k, v := range aMap {
						result[k] = v
					}
					for k, v := range bMap {
						result[k] = v
					}
					return result
				}
			}
			return b
		})
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		// For numeric types, use BinaryOperatorAggregate with add operator.
		// Pass reflect.Zero(fieldType).Interface() instead of fieldType itself
		// so createZeroValue uses the correct underlying numeric type for its
		// reflect.Zero call, not the reflect.Type descriptor.
		channel = NewBinaryOperatorAggregate(reflect.Zero(fieldType).Interface(), IntAdd)
	default:
		// Default to LastValue channel
		channel = NewLastValue(fieldType)
	}

	// Set the channel key
	channel.SetKey(fieldName)

	// If a reducer is provided, wrap the channel with it
	if reducer != nil {
		return NewReducerChannel(channel, reducer), nil
	}

	return channel, nil
}

// Built-in reducer functions for common types.
var (
	// AddReducer adds numeric values.
	AddReducer = func(current, update interface{}) interface{} {
		if current == nil {
			return update
		}
		// Try int addition
		if ci, ok := current.(int); ok {
			if ui, ok := update.(int); ok {
				return ci + ui
			}
		}
		// Try float64 addition
		if cf, ok := current.(float64); ok {
			if uf, ok := update.(float64); ok {
				return cf + uf
			}
		}
		// Fallback: return update
		return update
	}

	// AppendReducer appends to slices.
	AppendReducer = func(current, update interface{}) interface{} {
		if current == nil {
			return []interface{}{update}
		}
		if slice, ok := current.([]interface{}); ok {
			return append(slice, update)
		}
		// Convert to slice
		return []interface{}{current, update}
	}

	// MergeReducer merges maps.
	MergeReducer = func(current, update interface{}) interface{} {
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
		return update
	}
)

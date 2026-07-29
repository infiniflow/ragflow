package channels

import (
	"fmt"
	"reflect"

	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
)

// BinaryOperator is a function that combines two values into one.
type BinaryOperator func(a, b interface{}) interface{}

// BinaryOperatorAggregate stores the result of applying a binary operator to the current value and each new value.
type BinaryOperatorAggregate struct {
	BaseChannel
	value    interface{}
	operator BinaryOperator
}

// NewBinaryOperatorAggregate creates a new BinaryOperatorAggregate channel.
func NewBinaryOperatorAggregate(typ interface{}, operator BinaryOperator) *BinaryOperatorAggregate {
	c := &BinaryOperatorAggregate{
		BaseChannel: BaseChannel{Typ: typ},
		operator:    operator,
	}

	// Try to initialize with zero value
	c.value = createZeroValue(typ)

	return c
}

// createZeroValue creates a zero value for the given type.
func createZeroValue(typ interface{}) (result interface{}) {
	result = Missing
	if typ == nil {
		return
	}

	rt := reflect.TypeOf(typ)

	// Handle special collection types
	switch rt.String() {
	case "[]interface {}":
		return make([]interface{}, 0)
	case "map[string]interface {}":
		return make(map[string]interface{})
	}

	// Try to create instance
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}

	if rt.Kind() == reflect.Slice {
		return reflect.MakeSlice(rt, 0, 0).Interface()
	}

	if rt.Kind() == reflect.Map {
		return reflect.MakeMap(rt).Interface()
	}

	// Use named return so the deferred recovery can return Missing.
	defer func() {
		if r := recover(); r != nil {
			result = Missing
		}
	}()

	// Create a zero value using reflect.Zero
	zero := reflect.Zero(rt)
	result = zero.Interface()
	return
}

// Get returns the current value of the channel.
func (c *BinaryOperatorAggregate) Get() (interface{}, error) {
	if IsMissing(c.value) {
		return nil, &errors.EmptyChannelError{}
	}
	return c.value, nil
}

// IsAvailable returns true if the channel has a value.
func (c *BinaryOperatorAggregate) IsAvailable() bool {
	return !IsMissing(c.value)
}

// isOverwrite checks if a value is an overwrite wrapper.
func isOverwrite(value interface{}) (bool, interface{}) {
	if value == nil {
		return false, nil
	}

	// Check for Overwrite type
	type overwriter interface {
		GetValue() interface{}
	}
	if ow, ok := value.(overwriter); ok {
		return true, ow.GetValue()
	}

	// Check for map with __overwrite__ key
	if m, ok := value.(map[string]interface{}); ok {
		if len(m) == 1 {
			if v, exists := m[constants.Overwrite]; exists {
				return true, v
			}
		}
	}

	return false, nil
}

// Update updates the channel with values.
func (c *BinaryOperatorAggregate) Update(values []interface{}) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}

	// Initialize with first value if empty
	if IsMissing(c.value) {
		c.value = values[0]
		values = values[1:]
	}

	seenOverwrite := false
	for _, value := range values {
		isOver, overwriteValue := isOverwrite(value)
		if isOver {
			if seenOverwrite {
				return false, &errors.InvalidUpdateError{
					Message: "Can receive only one Overwrite value per super-step.",
				}
			}
			c.value = overwriteValue
			seenOverwrite = true
			continue
		}

		if !seenOverwrite {
			c.value = c.operator(c.value, value)
		}
	}

	return true, nil
}

// Copy returns a copy of the channel.
func (c *BinaryOperatorAggregate) Copy() Channel {
	newCh := NewBinaryOperatorAggregate(c.Typ, c.operator)
	newCh.Key = c.Key
	newCh.value = c.value
	return newCh
}

// Checkpoint returns the current value.
func (c *BinaryOperatorAggregate) Checkpoint() interface{} {
	return c.value
}

// FromCheckpoint restores the channel from a checkpoint.
func (c *BinaryOperatorAggregate) FromCheckpoint(checkpoint interface{}) Channel {
	newCh := NewBinaryOperatorAggregate(c.Typ, c.operator)
	newCh.Key = c.Key
	if !IsMissing(checkpoint) {
		newCh.value = checkpoint
	}
	return newCh
}

// String concatenation operator for BinaryOperatorAggregate.
func StringConcat(a, b interface{}) interface{} {
	sa, ok1 := a.(string)
	sb, ok2 := b.(string)
	if !ok1 || !ok2 {
		// Gracefully degrade: concatenate via fmt.Sprint.
		return fmt.Sprint(a) + fmt.Sprint(b)
	}
	return sa + sb
}

// IntAdd is an integer addition operator for BinaryOperatorAggregate.
// Supports int and float64. On type mismatch, returns a unchanged (graceful degradation).
func IntAdd(a, b interface{}) interface{} {
	if ai, ok := a.(int); ok {
		if bi, ok := b.(int); ok {
			return ai + bi
		}
	}
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok {
			return af + bf
		}
	}
	// Graceful degradation: return a unchanged.
	return a
}

// ListAppend appends two lists for BinaryOperatorAggregate.
func ListAppend(a, b interface{}) interface{} {
	if al, ok := a.([]interface{}); ok {
		if bl, ok := b.([]interface{}); ok {
			result := make([]interface{}, len(al)+len(bl))
			copy(result, al)
			copy(result[len(al):], bl)
			return result
		}
		result := make([]interface{}, len(al)+1)
		copy(result, al)
		result[len(al)] = b
		return result
	}
	return []interface{}{a, b}
}

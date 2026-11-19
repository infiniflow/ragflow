//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// UnmarshalCheckpointJSON restores a checkpoint without rounding dynamic integer
// IDs outside float64's safe range. Ordinary dynamic numbers remain float64;
// statically typed fields keep the types selected by encoding/json.
func UnmarshalCheckpointJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if len(bytes.TrimSpace(data[dec.InputOffset():])) != 0 {
		return fmt.Errorf("checkpoint: unexpected data after JSON value")
	}
	return restoreCheckpointNumbers(reflect.ValueOf(target).Elem())
}

// restoreCheckpointNumbers visits interface values as well as typed containers:
// parallel inputs can be structs or typed maps containing dynamic JSON values.
func restoreCheckpointNumbers(value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		if number, ok := value.Interface().(json.Number); ok {
			const maxSafeInteger = "9007199254740991" // 2^53 - 1
			magnitude := strings.TrimPrefix(number.String(), "-")
			if !strings.ContainsAny(magnitude, ".eE") && (len(magnitude) > len(maxSafeInteger) || len(magnitude) == len(maxSafeInteger) && magnitude > maxSafeInteger) {
				return nil
			}
			normalized, err := number.Float64()
			if err != nil {
				return fmt.Errorf("checkpoint: decode number %q: %w", number, err)
			}
			value.Set(reflect.ValueOf(normalized))
			return nil
		}
		// A concrete value held in an interface is not settable. Copy it before
		// traversing so nested structs and interface numbers can be updated.
		item := reflect.New(value.Elem().Type()).Elem()
		item.Set(value.Elem())
		if err := restoreCheckpointNumbers(item); err != nil {
			return err
		}
		value.Set(item)
	case reflect.Pointer:
		if !value.IsNil() {
			return restoreCheckpointNumbers(value.Elem())
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			item := reflect.New(value.Type().Elem()).Elem()
			item.Set(iter.Value())
			if err := restoreCheckpointNumbers(item); err != nil {
				return err
			}
			value.SetMapIndex(iter.Key(), item)
		}
	case reflect.Slice, reflect.Array:
		switch value.Type().Elem().Kind() {
		case reflect.Interface, reflect.Map, reflect.Struct, reflect.Pointer, reflect.Slice, reflect.Array:
			for i := 0; i < value.Len(); i++ {
				if err := restoreCheckpointNumbers(value.Index(i)); err != nil {
					return err
				}
			}
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanSet() {
				if err := restoreCheckpointNumbers(value.Field(i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

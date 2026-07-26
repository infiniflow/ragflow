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

// Package io — small type-coercion helpers shared by docs_generator.go
// and the per-format writers. Kept here (rather than in the parent
// component package) so the io/ subpackage has zero coupling to the
// canvas engine and can be tested in isolation.
package io

// stringFrom extracts a string from a conf map, returning the value
// and ok=true. nil / wrong-type yields ("", false).
func stringFrom(conf map[string]any, key string) (string, bool) {
	if conf == nil {
		return "", false
	}
	v, ok := conf[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// intFrom extracts an int from a conf map. JSON-decoded numbers
// commonly come in as float64; we accept both shapes for friendliness.
func intFrom(conf map[string]any, key string) (int, bool) {
	if conf == nil {
		return 0, false
	}
	v, ok := conf[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// boolFrom extracts a bool from a conf map.
func boolFrom(conf map[string]any, key string) (bool, bool) {
	if conf == nil {
		return false, false
	}
	v, ok := conf[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

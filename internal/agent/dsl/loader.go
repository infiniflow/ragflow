// Package dsl — version auto-detection loader.
//
// Loader accepts raw JSON bytes that may be either v1 (the legacy Python-era
// format) or v2 (the Go-native schema) and returns a uniform v2 *Canvas.
//
// Detection rules (in order):
//  1. Top-level "version" field == 2 -> V2.
//  2. Top-level "components" map whose values have an "obj" sub-object
//     with "component_name" -> V1.
//  3. Anything else -> error "unknown DSL version".
package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Version is the DSL schema version a payload was written in.
type Version int

const (
	// V1 is the legacy Python-era schema with the `obj` wrapper and
	// deprecated param fields. See plan §2.11.7.
	V1 Version = 1
	// V2 is the Go-native flat schema. See plan §4.6.
	V2 Version = 2
)

// String renders a Version for diagnostic messages.
func (v Version) String() string {
	switch v {
	case V1:
		return "v1"
	case V2:
		return "v2"
	default:
		return fmt.Sprintf("v?(%d)", int(v))
	}
}

// DetectVersion peeks at the JSON bytes and returns the schema version.
//
// Detection is a structural probe — it does not perform a full unmarshal.
// A payload is reported as V2 only if the top-level integer field
// `"version"` equals 2. A payload is reported as V1 if it has a top-level
// `"components"` map whose values each contain an `"obj"` sub-object with
// a `"component_name"` string field. Anything else is rejected.
func DetectVersion(raw []byte) (Version, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	// Probe 1: top-level "version": 2 -> V2.
	var v2Probe struct {
		Version int `json:"version"`
	}
	if err := dec.Decode(&v2Probe); err != nil {
		return 0, fmt.Errorf("dsl: detect version: %w", err)
	}
	if v2Probe.Version == CurrentVersion {
		return V2, nil
	}

	// Probe 2: top-level "components" with `obj.component_name` -> V1.
	dec2 := json.NewDecoder(bytes.NewReader(raw))
	var v1Probe struct {
		Components map[string]json.RawMessage `json:"components"`
	}
	if err := dec2.Decode(&v1Probe); err != nil {
		return 0, fmt.Errorf("dsl: detect version: %w", err)
	}
	if len(v1Probe.Components) == 0 {
		return 0, fmt.Errorf("dsl: unknown DSL version (no top-level version and no components map)")
	}
	for _, raw := range v1Probe.Components {
		var objProbe struct {
			Obj struct {
				ComponentName string `json:"component_name"`
			} `json:"obj"`
		}
		if err := json.Unmarshal(raw, &objProbe); err != nil {
			return 0, fmt.Errorf("dsl: detect version: probe v1: %w", err)
		}
		if objProbe.Obj.ComponentName != "" {
			return V1, nil
		}
	}
	return 0, fmt.Errorf("dsl: unknown DSL version (components map has no obj.component_name)")
}

// Load auto-detects the version of raw and returns a v2 Canvas.
//
// V1 payloads are run through v1ToV2 first; v2 payloads are unmarshaled
// directly via UnmarshalV2.
func Load(raw []byte) (*Canvas, error) {
	v, err := DetectVersion(raw)
	if err != nil {
		return nil, err
	}
	switch v {
	case V1:
		return LoadV1(raw)
	case V2:
		return LoadV2(raw)
	default:
		return nil, fmt.Errorf("dsl: unsupported version %s", v)
	}
}

// LoadV1 parses a v1 payload and converts it to a v2 Canvas. Returns
// validation errors for v1-only consumers (e.g. integration tests).
func LoadV1(raw []byte) (*Canvas, error) {
	c, err := v1ToV2(raw)
	if err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("dsl: v1->v2 validation: %w", err)
	}
	return c, nil
}

// LoadV2 parses a v2 payload and validates it.
func LoadV2(raw []byte) (*Canvas, error) {
	return UnmarshalV2(raw)
}

// DecodeReader is a convenience: reads a JSON byte stream and routes it
// through Load. The full body is buffered in memory.
func DecodeReader(r io.Reader) (*Canvas, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, fmt.Errorf("dsl: read: %w", err)
	}
	return Load(buf.Bytes())
}

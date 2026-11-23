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

//go:build !ee

package table

import (
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestTableItemVHExtensionFieldsExist is the RED guard for the new TableItem
// extension fields. Reflection keeps this test compiling before the fields
// exist, so it fails on a real assertion instead of a missing symbol.
func TestTableItemVHExtensionFieldsExist(t *testing.T) {
	typ := reflect.TypeOf(pdf.TableItem{})

	f, ok := typ.FieldByName("GridUnified")
	if !ok {
		t.Fatal("TableItem.GridUnified is missing")
	}
	if f.Type.Kind() != reflect.Bool {
		t.Errorf("TableItem.GridUnified kind = %v, want bool", f.Type.Kind())
	}

	f, ok = typ.FieldByName("VHPageSpans")
	if !ok {
		t.Fatal("TableItem.VHPageSpans is missing")
	}
	if f.Type.Kind() != reflect.Slice {
		t.Fatalf("TableItem.VHPageSpans kind = %v, want slice", f.Type.Kind())
	}
	if got := f.Type.Elem().Name(); got != "VHPageSpan" {
		t.Errorf("TableItem.VHPageSpans element type = %q, want VHPageSpan", got)
	}

	span := f.Type.Elem()
	for _, name := range []string{"Page", "CellStart", "CellEnd", "Scale", "CropOffX", "CropOffY"} {
		sf, ok := span.FieldByName(name)
		if !ok {
			t.Errorf("VHPageSpan.%s is missing", name)
			continue
		}
		switch name {
		case "Page", "CellStart", "CellEnd":
			if sf.Type.Kind() != reflect.Int {
				t.Errorf("VHPageSpan.%s kind = %v, want int", name, sf.Type.Kind())
			}
		default:
			if sf.Type.Kind() != reflect.Float64 {
				t.Errorf("VHPageSpan.%s kind = %v, want float64", name, sf.Type.Kind())
			}
		}
	}
}

// setVHExtensionFields fills the new fields through reflection so the test
// source compiles before the fields exist. It reports whether they were set.
func setVHExtensionFields(item *pdf.TableItem, gridUnified bool, span ...[6]float64) bool {
	v := reflect.ValueOf(item).Elem()
	gf := v.FieldByName("GridUnified")
	sf := v.FieldByName("VHPageSpans")
	if !gf.IsValid() || !sf.IsValid() || !gf.CanSet() || !sf.CanSet() {
		return false
	}
	gf.SetBool(gridUnified)
	if len(span) == 0 {
		return true
	}
	slice := reflect.MakeSlice(sf.Type(), len(span), len(span))
	for i, s := range span {
		e := slice.Index(i)
		e.FieldByName("Page").SetInt(int64(s[0]))
		e.FieldByName("CellStart").SetInt(int64(s[1]))
		e.FieldByName("CellEnd").SetInt(int64(s[2]))
		e.FieldByName("Scale").SetFloat(s[3])
		e.FieldByName("CropOffX").SetFloat(s[4])
		e.FieldByName("CropOffY").SetFloat(s[5])
	}
	sf.Set(slice)
	return true
}

// TestConstructTableVHPageSpansRoutesToStub checks that a RenderVH table with
// per-page spans stays on the stub path (empty output) and never falls through
// to the core renderer.
func TestConstructTableVHPageSpansRoutesToStub(t *testing.T) {
	item := &pdf.TableItem{Grid: vhDispatchGrid(), RenderVH: true}
	if !setVHExtensionFields(item, false, [6]float64{1, 0, 2, 1.0, 5, 7}) {
		t.Fatal("could not set VH extension fields")
	}
	if got := ConstructTable(nil, nil, "", item); got != "" {
		t.Errorf("RenderVH+VHPageSpans must route to the stub (empty), got %q", got)
	}
}

// TestConstructTableVHGridUnifiedRoutesToStub checks that a RenderVH table with
// a unified grid stays on the stub path (empty output).
func TestConstructTableVHGridUnifiedRoutesToStub(t *testing.T) {
	item := &pdf.TableItem{Grid: vhDispatchGrid(), RenderVH: true}
	if !setVHExtensionFields(item, true) {
		t.Fatal("could not set GridUnified field")
	}
	if got := ConstructTable(nil, nil, "", item); got != "" {
		t.Errorf("RenderVH+GridUnified must route to the stub (empty), got %q", got)
	}
}

// TestVHExtensionFieldsZeroByDefault locks that the new fields are inert for
// the default pipeline: nothing in this tree sets them.
func TestVHExtensionFieldsZeroByDefault(t *testing.T) {
	v := reflect.ValueOf(pdf.TableItem{})
	gf := v.FieldByName("GridUnified")
	if !gf.IsValid() {
		t.Fatal("TableItem.GridUnified is missing")
	}
	if gf.Bool() {
		t.Error("GridUnified should default to false")
	}
	sf := v.FieldByName("VHPageSpans")
	if !sf.IsValid() {
		t.Fatal("TableItem.VHPageSpans is missing")
	}
	if sf.Len() != 0 {
		t.Errorf("VHPageSpans should default to empty, got %d", sf.Len())
	}
}

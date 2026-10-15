package common

import (
	"reflect"
	"sort"
	"strings"
)

// The two ways a dataset config names a compilation model.
//
// A config may point at a model group, which resolves to every template inside
// it, or at one template directly. Both are real — the compiler accepts either
// (ParseParam) — so a reference model that understood only one of them would
// answer "no dataset is affected" for a model that is in fact in use, which is
// the one answer an impact analysis must never give.
const (
	ModelRefGroup    = "group"
	ModelRefTemplate = "template"
)

// ModelRef identifies one compilation model a dataset config refers to.
type ModelRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// templateRefKeys maps the config keys that name a compilation model to the
// kind of reference they carry. The plural spellings are accepted with a single
// id as well: the editors have written a bare string into the plural key, and a
// reference that is silently dropped is worse than one accepted twice.
var templateRefKeys = map[string]string{
	"compilation_template_group_id":  ModelRefGroup,
	"compilation_template_group_ids": ModelRefGroup,
	"compilation_template_id":        ModelRefTemplate,
	"compilation_template_ids":       ModelRefTemplate,
}

// TemplateRefsFromConfig lists every compilation model a dataset config names,
// deduplicated and ordered.
//
// The walk covers the whole document instead of reading a known key path: the
// compiler looks for its parameters wherever they sit — at the dataset level or
// nested inside a canvas component node — and the reference index has to find
// exactly what the compiler would find. Named map types are walked too, so a
// value that has not been through a JSON round trip behaves the same.
func TemplateRefsFromConfig(config interface{}) []ModelRef {
	out := make([]ModelRef, 0, 2)
	collectTemplateRefs(reflect.ValueOf(config), &out)
	return dedupeModelRefs(out)
}

func collectTemplateRefs(value reflect.Value, out *[]ModelRef) {
	value = derefValue(value)
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return
		}
		for _, key := range value.MapKeys() {
			child := value.MapIndex(key)
			if kind, isRef := templateRefKeys[key.String()]; isRef {
				*out = append(*out, refsInValue(child, kind)...)
				continue
			}
			collectTemplateRefs(child, out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectTemplateRefs(value.Index(i), out)
		}
	}
}

// refsInValue reads the ids out of one reference key's value, which holds a
// single id or a list of them.
func refsInValue(value reflect.Value, kind string) []ModelRef {
	value = derefValue(value)
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		if id := strings.TrimSpace(value.String()); id != "" {
			return []ModelRef{{Kind: kind, ID: id}}
		}
	case reflect.Slice, reflect.Array:
		out := make([]ModelRef, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			item := derefValue(value.Index(i))
			if !item.IsValid() || item.Kind() != reflect.String {
				continue
			}
			if id := strings.TrimSpace(item.String()); id != "" {
				out = append(out, ModelRef{Kind: kind, ID: id})
			}
		}
		return out
	}
	return nil
}

func derefValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func dedupeModelRefs(refs []ModelRef) []ModelRef {
	seen := make(map[ModelRef]struct{}, len(refs))
	out := make([]ModelRef, 0, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

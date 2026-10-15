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

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/utility"

	"gopkg.in/yaml.v3"
)

// fillConfigDefaultLLM mirrors Python CompilationTemplateService.
// fill_config_default_llm: when the config has no explicit llm_id it lazily
// fills the tenant's default chat model id (read-side only; the DB row is left
// untouched).
func fillConfigDefaultLLM(ctx context.Context, tenantDAO *dao.TenantDAO, config entity.JSONMap, tenantID *string) entity.JSONMap {
	if len(config) == 0 || tenantID == nil || *tenantID == "" {
		return config
	}
	if _, ok := config["llm_id"]; ok {
		return config
	}
	tenant, err := tenantDAO.GetByID(ctx, dao.DB, *tenantID)
	if err != nil || tenant == nil || tenant.TenantLLMID == nil {
		return config
	}
	out := make(entity.JSONMap, len(config)+1)
	for k, v := range config {
		out[k] = v
	}
	out["llm_id"] = *tenant.TenantLLMID
	return out
}

// capitalizeTitle capitalizes the first letter of a lowercase section name for
// error messages (replaces deprecated strings.Title).
func capitalizeTitle(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// TemplateListItem is the read-side representation of a compilation template,
// mirroring Python CompilationTemplateService._to_saved_dict.
type TemplateListItem struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Kind        string         `json:"kind"`
	Config      entity.JSONMap `json:"config"`
	CreateTime  string         `json:"create_time,omitempty"`
	UpdateTime  string         `json:"update_time,omitempty"`
}

// BuiltinTemplate is the palette representation of a built-in template,
// mirroring Python CompilationTemplateService._to_builtin_dict.
type BuiltinTemplate struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	DisplayName string         `json:"display_name"`
	Description string         `json:"description"`
	Config      entity.JSONMap `json:"config"`
}

// WikiPreset is a wiki page-structure preset loaded from YAML, mirroring
// Python load_wiki_presets_from_files.
type WikiPreset struct {
	ID          string `json:"id"`
	Topic       string `json:"topic"`
	Instruction string `json:"instruction"`
	Example     string `json:"example"`
}

// CompilationTemplateService implements the read-side compilation template
// operations (read-only YAML palette + wiki presets) used by the REST APIs.
type CompilationTemplateService struct {
	tenantDAO *dao.TenantDAO
}

// NewCompilationTemplateService creates a CompilationTemplateService.
func NewCompilationTemplateService() *CompilationTemplateService {
	return &CompilationTemplateService{
		tenantDAO: dao.NewTenantDAO(),
	}
}

// ListBuiltins returns the read-only template palette loaded from the YAML
// definitions. Built-in templates are never persisted in the database.
func (s *CompilationTemplateService) ListBuiltins(ctx context.Context, tenantID string) ([]*BuiltinTemplate, error) {
	templates, err := loadBuiltinTemplates()
	if err != nil {
		return nil, err
	}
	tenantPtr := &tenantID
	for _, template := range templates {
		template.Config = fillConfigDefaultLLM(ctx, s.tenantDAO, template.Config, tenantPtr)
	}
	return s.sortBuiltins(templates), nil
}

// LoadWikiPresets loads the wiki page-structure presets from the
func (s *CompilationTemplateService) LoadWikiPresets() ([]*WikiPreset, error) {
	wikiDir := filepath.Join(utility.GetProjectBaseDirectory(), "internal", "ingestion", "knowledge_compile", "templates", "wiki")
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*WikiPreset{}, nil
		}
		return nil, err
	}
	var presets []*WikiPreset
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		path := filepath.Join(wikiDir, entry.Name())
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}
		var doc map[string]interface{}
		if yerr := yaml.Unmarshal(data, &doc); yerr != nil || doc == nil {
			continue
		}
		presets = append(presets, &WikiPreset{
			ID:          strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			Topic:       strings.TrimSpace(yamlStr(doc["topic"])),
			Instruction: yamlStr(doc["instruction"]),
			Example:     yamlStr(doc["example"]),
		})
	}
	return presets, nil
}

// ontologyDatatypeKinds are the datatype ranges an ontology template may
// declare for a datatype property. The declared range decides which JSON value
// the projection writes into the entity row's `attr` column, so an unknown one
// has to be rejected at save time rather than silently mis-stored later.
var ontologyDatatypeKinds = map[string]bool{
	"string": true, "list": true,
	"int": true, "integer": true,
	"float": true, "number": true,
	"bool": true, "boolean": true,
	"date": true,
}

// validateOntologyTemplate checks the cross-references a standard ontology
// template carries: rdfs:subClassOf (class `parent`), and a property's
// `domain` / `range` / `datatype`. Every one of them is a reference from one
// field to another, so none can be checked inside the per-field loop.
//
// A dangling reference is not a cosmetic problem: the projection flips edges
// whose endpoints do not match the declaration, so a typo in `range` silently
// kills every edge of that property (see ontology.md §8).
func validateOntologyTemplate(configMap map[string]interface{}) error {
	fieldsOf := func(section string) []map[string]interface{} {
		sec, _ := configMap[section].(map[string]interface{})
		raw, _ := sec["fields"].([]interface{})
		out := make([]map[string]interface{}, 0, len(raw))
		for _, f := range raw {
			if fm, ok := f.(map[string]interface{}); ok {
				out = append(out, fm)
			}
		}
		return out
	}

	classes := map[string]bool{}
	parentOf := map[string][]string{}
	for _, f := range fieldsOf("entity") {
		name := strings.TrimSpace(yamlStr(f["type"]))
		if name == "" {
			continue
		}
		classes[name] = true
		if parents := splitTypeList(yamlStr(f["parent"])); len(parents) > 0 {
			parentOf[name] = parents
		}
	}
	if len(classes) == 0 {
		return errors.New("ontology template must declare at least one class")
	}

	// A class may declare several superclasses (`|`-separated), so both checks
	// below walk the whole parent set rather than one chain.
	for child, parents := range parentOf {
		for _, p := range parents {
			if !classes[p] {
				return fmt.Errorf("class %q declares unknown parent %q", child, p)
			}
		}
	}
	// A cyclic parent chain would make the inherited-attribute walk used by the
	// ontology queries (class -> all its attributes) never terminate.
	for child := range parentOf {
		seen := map[string]bool{}
		queue := append([]string{}, parentOf[child]...)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == "" || seen[cur] {
				continue
			}
			if cur == child {
				return fmt.Errorf("class %q has a cyclic parent chain", child)
			}
			seen[cur] = true
			queue = append(queue, parentOf[cur]...)
		}
	}

	for _, f := range fieldsOf("relation") {
		name := strings.TrimSpace(yamlStr(f["type"]))
		if name == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(yamlStr(f["kind"])))
		domain := strings.TrimSpace(yamlStr(f["domain"]))
		rangeRaw := strings.TrimSpace(yamlStr(f["range"]))
		datatype := strings.ToLower(strings.TrimSpace(yamlStr(f["datatype"])))

		if kind == "" {
			// Infer from the declaration: a range naming a declared class is an
			// object property, anything else a datatype property.
			kind = "datatype"
			for _, r := range splitTypeList(rangeRaw) {
				if classes[r] {
					kind = "object"
					break
				}
			}
		}

		switch kind {
		case "object":
			if len(splitTypeList(domain)) == 0 {
				return fmt.Errorf("object property %q declares no domain", name)
			}
			if len(splitTypeList(rangeRaw)) == 0 {
				return fmt.Errorf("object property %q declares no range", name)
			}
			for _, d := range splitTypeList(domain) {
				if !classes[d] {
					return fmt.Errorf("property %q declares unknown domain class %q", name, d)
				}
			}
			for _, r := range splitTypeList(rangeRaw) {
				if !classes[r] {
					return fmt.Errorf("property %q declares unknown range class %q", name, r)
				}
			}
		case "datatype":
			if len(splitTypeList(domain)) == 0 {
				return fmt.Errorf("datatype property %q declares no domain", name)
			}
			for _, d := range splitTypeList(domain) {
				if !classes[d] {
					return fmt.Errorf("property %q declares unknown domain class %q", name, d)
				}
			}
			if datatype != "" && !ontologyDatatypeKinds[datatype] {
				return fmt.Errorf("property %q declares unsupported datatype %q", name, datatype)
			}
		default:
			return fmt.Errorf("property %q has invalid kind %q (want object or datatype)", name, kind)
		}
	}
	return nil
}

// ValidateTemplatePayload validates a single template payload, mirroring the
// Python compilation_template_validation module. It returns an error describing
// the first problem found.
func ValidateTemplatePayload(req map[string]interface{}, requireAll bool) error {
	if requireAll {
		for _, key := range []string{"name", "kind", "config"} {
			if _, ok := req[key]; !ok {
				return fmt.Errorf("missing required field: %s", key)
			}
		}
	}
	if name, ok := req["name"]; ok {
		nameStr, ok2 := name.(string)
		if !ok2 || strings.TrimSpace(nameStr) == "" || len([]byte(nameStr)) > 128 {
			return errors.New("invalid template name")
		}
	}
	if desc, ok := req["description"]; ok {
		if descStr, ok2 := desc.(string); !ok2 || utf8.RuneCountInString(descStr) > 1024 {
			return errors.New("invalid template description")
		}
	}
	if kind, ok := req["kind"]; ok {
		if kindStr, ok2 := kind.(string); !ok2 || kindStr == "" {
			return errors.New("invalid template kind")
		}
	}
	config, hasConfig := req["config"]
	if hasConfig {
		// config may arrive as map[string]interface{} (raw payloads) or as
		// entity.JSONMap (payloads built from a typed GroupTemplate, whose Config
		// field is JSONMap). A bare type assertion on the former would reject the
		// latter because JSONMap is a distinct named type, so normalize both.
		var configMap map[string]interface{}
		switch c := config.(type) {
		case map[string]interface{}:
			configMap = c
		case entity.JSONMap:
			configMap = map[string]interface{}(c)
		default:
			return errors.New("invalid template config")
		}
		if utf8.RuneCountInString(yamlStr(configMap["global_rules"])) > 4096 {
			return errors.New("global compilation rules is too long")
		}
		for _, section := range []string{"entity", "relation"} {
			sec, _ := configMap[section].(map[string]interface{})
			fields, _ := sec["fields"].([]interface{})
			seen := map[string]struct{}{}
			for _, f := range fields {
				fm, _ := f.(map[string]interface{})
				fieldType := strings.TrimSpace(yamlStr(fm["type"]))
				if fieldType == "" {
					return fmt.Errorf("%s type is required", capitalizeTitle(section))
				}
				if _, dup := seen[fieldType]; dup {
					return fmt.Errorf("%s type can not be duplicated", capitalizeTitle(section))
				}
				seen[fieldType] = struct{}{}
				if strings.TrimSpace(yamlStr(fm["description"])) == "" {
					return fmt.Errorf("%s field description is required", capitalizeTitle(section))
				}
				if utf8.RuneCountInString(yamlStr(fm["description"])) > 1024 {
					return fmt.Errorf("%s field description is too long", capitalizeTitle(section))
				}
				if utf8.RuneCountInString(yamlStr(fm["rule"])) > 1024 {
					return fmt.Errorf("%s field rule is too long", capitalizeTitle(section))
				}
			}
		}
		// Ontology templates additionally carry cross-references between fields
		// (class parent, property domain / range / datatype), which the per-field
		// loop above cannot see.
		if kccommon.IsOntologyKind(yamlStr(configMap["kind"])) || kccommon.IsOntologyKind(yamlStr(req["kind"])) {
			if err := validateOntologyTemplate(configMap); err != nil {
				return err
			}
		}
		if configMap["kind"] == "wiki" || req["kind"] == "wiki" {
			for _, group := range []string{"claim", "concept"} {
				sec, _ := configMap[group].(map[string]interface{})
				fields, _ := sec["fields"].([]interface{})
				for _, f := range fields {
					fm, _ := f.(map[string]interface{})
					switch group {
					case "claim":
						if strings.TrimSpace(yamlStr(fm["statement"])) == "" {
							return errors.New("claim statement is required")
						}
						if strings.TrimSpace(yamlStr(fm["subject"])) == "" {
							return errors.New("claim subject is required")
						}
						if utf8.RuneCountInString(yamlStr(fm["statement"])) > 1024 {
							return errors.New("claim statement is too long")
						}
						if utf8.RuneCountInString(yamlStr(fm["subject"])) > 1024 {
							return errors.New("claim subject is too long")
						}
					case "concept":
						if strings.TrimSpace(yamlStr(fm["term"])) == "" {
							return errors.New("concept term is required")
						}
						if strings.TrimSpace(yamlStr(fm["definition_excerpt"])) == "" {
							return errors.New("concept definition excerpt is required")
						}
						if utf8.RuneCountInString(yamlStr(fm["term"])) > 1024 {
							return errors.New("concept term is too long")
						}
						if utf8.RuneCountInString(yamlStr(fm["definition_excerpt"])) > 1024 {
							return errors.New("concept definition excerpt is too long")
						}
					}
				}
			}
		}
	}
	return nil
}

// sortBuiltins mirrors Python _sort_builtins: empty-kind entries first, then by
// display name.
func (s *CompilationTemplateService) sortBuiltins(templates []*BuiltinTemplate) []*BuiltinTemplate {
	sort.SliceStable(templates, func(i, j int) bool {
		emptyI := templates[i].Kind == "empty" || templates[i].ID == "empty"
		emptyJ := templates[j].Kind == "empty" || templates[j].ID == "empty"
		if emptyI != emptyJ {
			return emptyI
		}
		return strings.ToLower(templates[i].DisplayName) < strings.ToLower(templates[j].DisplayName)
	})
	return templates
}

func loadBuiltinTemplates() ([]*BuiltinTemplate, error) {
	dir := filepath.Join(utility.GetProjectBaseDirectory(), "internal", "ingestion", "knowledge_compile", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*BuiltinTemplate{}, nil
		}
		return nil, err
	}
	templates := make([]*BuiltinTemplate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if rerr != nil {
			continue
		}
		var doc struct {
			Kind        string                 `yaml:"kind"`
			DisplayName string                 `yaml:"display_name"`
			Description string                 `yaml:"description"`
			Config      map[string]interface{} `yaml:"config"`
		}
		if yerr := yaml.Unmarshal(data, &doc); yerr != nil {
			continue
		}
		if doc.Kind == "" || doc.DisplayName == "" || doc.Config == nil {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		templates = append(templates, &BuiltinTemplate{
			ID:          id,
			Kind:        doc.Kind,
			DisplayName: doc.DisplayName,
			Description: doc.Description,
			Config:      entity.JSONMap(doc.Config),
		})
	}
	return templates, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// yamlStr safely stringifies a YAML value, returning "" when the key is absent
// (instead of fmt.Sprint(nil) which yields "<nil>").
func yamlStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

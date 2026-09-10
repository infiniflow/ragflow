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
	"ragflow/internal/utility"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
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
// operations (builtins palette + wiki presets) used by the REST APIs.
type CompilationTemplateService struct {
	templateDAO *dao.CompilationTemplateDAO
	tenantDAO   *dao.TenantDAO
}

// NewCompilationTemplateService creates a CompilationTemplateService.
func NewCompilationTemplateService() *CompilationTemplateService {
	return &CompilationTemplateService{
		templateDAO: dao.NewCompilationTemplateDAO(),
		tenantDAO:   dao.NewTenantDAO(),
	}
}

// ListBuiltins returns the built-in template palette with the tenant's default
// LLM id lazily filled in, mirroring the Python list_builtins blueprint flow
// (list from DB; if empty, seed from files and retry; then fill default LLM).
func (s *CompilationTemplateService) ListBuiltins(ctx context.Context, tenantID string) ([]*BuiltinTemplate, error) {
	templates, err := s.templateDAO.ListBuiltins(ctx, dao.DB)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		if err = s.seedBuiltins(ctx); err != nil {
			return nil, err
		}
		templates, err = s.templateDAO.ListBuiltins(ctx, dao.DB)
		if err != nil {
			return nil, err
		}
	}
	out := make([]*BuiltinTemplate, 0, len(templates))
	for _, t := range templates {
		config := fillConfigDefaultLLM(ctx, s.tenantDAO, t.Config, t.TenantID)
		out = append(out, &BuiltinTemplate{
			ID:          t.ID,
			Kind:        t.Kind,
			DisplayName: t.Name,
			Description: derefString(t.Description),
			Config:      config,
		})
	}
	return s.sortBuiltins(out), nil
}

// LoadWikiPresets loads the wiki page-structure presets from the
// init_data/compilation_templates/wiki/*.yaml files, filesystem-fresh per call.
func (s *CompilationTemplateService) LoadWikiPresets() ([]*WikiPreset, error) {
	wikiDir := filepath.Join(utility.GetProjectBaseDirectory(),
		"api", "db", "init_data", "compilation_templates", "wiki")
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

// seedBuiltins loads the built-in templates from the init_data YAML files and
// upserts them (as a safety net when the DB has no built-ins yet), mirroring
// Python seed_builtins_from_files.
func (s *CompilationTemplateService) seedBuiltins(ctx context.Context) error {
	dir := filepath.Join(utility.GetProjectBaseDirectory(),
		"api", "db", "init_data", "compilation_templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // directory absent is acceptable; nothing to seed
	}
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
		desc := doc.Description
		valid := string(entity.StatusValid)
		tmpl := &entity.CompilationTemplate{
			ID:        id,
			Name:      doc.DisplayName,
			Kind:      doc.Kind,
			Config:    entity.JSONMap(doc.Config),
			IsBuiltin: true,
			Status:    &valid,
		}
		if doc.Description != "" {
			tmpl.Description = &desc
		}
		_ = s.upsertBuiltin(ctx, tmpl)
	}
	return nil
}

func (s *CompilationTemplateService) upsertBuiltin(ctx context.Context, t *entity.CompilationTemplate) error {
	existing, err := s.templateDAO.GetByID(ctx, dao.DB, t.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.templateDAO.Save(ctx, dao.DB, t)
	}
	if err != nil {
		return err
	}
	m := map[string]interface{}{
		"name": t.Name, "kind": t.Kind, "config": t.Config,
		"description": t.Description, "is_builtin": true,
		"status": t.Status,
	}
	return s.templateDAO.UpdateFields(ctx, dao.DB, existing.ID, m)
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

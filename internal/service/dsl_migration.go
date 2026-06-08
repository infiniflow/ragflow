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
	"reflect"
	"regexp"
	"sort"
	"strings"

	"ragflow/internal/entity"
)

type componentRenameRule struct {
	oldName string
	newName string
}

var componentRenameRules = []componentRenameRule{
	{oldName: "Splitter", newName: "TokenChunker"},
	{oldName: "HierarchicalMerger", newName: "TitleChunker"},
	{oldName: "PDFGenerator", newName: "DocGenerator"},
}

var componentRenames = buildComponentRenameMap(componentRenameRules)

func buildComponentRenameMap(rules []componentRenameRule) map[string]string {
	res := make(map[string]string, len(rules))
	for _, rule := range rules {
		res[rule.oldName] = rule.newName
	}
	return res
}

var nodeTypeRenames = map[string]string{
	"splitterNode": "chunkerNode",
}

var variableRefPattern = regexp.MustCompile(`(\{+\s*)([A-Za-z0-9:_-]+)(@[A-Za-z0-9_.-]+)(\s*\}+)`)

func deepCopyJSONLike(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		res := make(map[string]interface{}, len(v))
		for k, item := range v {
			res[k] = deepCopyJSONLike(item)
		}
		return res

	case []interface{}:
		res := make([]interface{}, len(v))
		for i, item := range v {
			res[i] = deepCopyJSONLike(item)
		}
		return res
	}

	rv := reflect.ValueOf(val)
	if !rv.IsValid() {
		return val
	}

	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return val
		}
		res := make(map[string]interface{}, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			res[iter.Key().String()] = deepCopyJSONLike(iter.Value().Interface())
		}
		return res

	case reflect.Slice, reflect.Array:
		res := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			res[i] = deepCopyJSONLike(rv.Index(i).Interface())
		}
		return res
	}

	return val
}

// NormalizeChunkerDSL rewrites legacy chunker component names and ids into the current DSL schema.
// This is intentionally a pure migration step that only rewrites structural identifiers.
func NormalizeChunkerDSL(dsl entity.JSONMap) entity.JSONMap {
	if dsl == nil {
		return dsl
	}

	// Deep copy to avoid mutating the original.
	// Avoid json.Marshal/json.Unmarshal here because it may turn int values into float64.
	normalized := make(entity.JSONMap, len(dsl))
	for k, v := range dsl {
		normalized[k] = deepCopyJSONLike(v)
	}

	componentsInter, ok := normalized["components"]
	if !ok {
		return normalized
	}
	components, ok := componentsInter.(map[string]interface{})
	if !ok {
		return normalized
	}

	componentIDMap := make(map[string]string)
	for componentID := range components {
		newComponentID := componentID
		for _, rule := range componentRenameRules {
			prefix := rule.oldName + ":"
			if strings.HasPrefix(componentID, prefix) {
				newComponentID = rule.newName + ":" + strings.TrimPrefix(componentID, prefix)
				break
			}
		}
		componentIDMap[componentID] = newComponentID
	}

	rewriteVariableRefs := func(text string) string {
		if newID, exists := componentIDMap[text]; exists {
			return newID
		}
		return variableRefPattern.ReplaceAllStringFunc(text, func(match string) string {
			submatches := variableRefPattern.FindStringSubmatch(match)
			if len(submatches) != 5 {
				return match
			}
			componentID := submatches[2]
			newID := componentID
			if mappedID, ok := componentIDMap[componentID]; ok {
				newID = mappedID
			}
			return submatches[1] + newID + submatches[3] + submatches[4]
		})
	}

	var rewriteValue func(val interface{}) interface{}
	rewriteValue = func(val interface{}) interface{} {
		switch v := val.(type) {
		case string:
			return rewriteVariableRefs(v)
		case []interface{}:
			res := make([]interface{}, len(v))
			for i, item := range v {
				res[i] = rewriteValue(item)
			}
			return res
		case map[string]interface{}:
			res := make(map[string]interface{})
			for k, item := range v {
				res[k] = rewriteValue(item)
			}
			return res
		default:
			return v
		}
	}

	rewrittenComponents := make(map[string]interface{})
	for oldComponentID, compInter := range components {
		newComponentID := componentIDMap[oldComponentID]
		newComponentInter := rewriteValue(compInter)

		if newComponent, ok := newComponentInter.(map[string]interface{}); ok {
			if objInter, ok := newComponent["obj"]; ok {
				if obj, ok := objInter.(map[string]interface{}); ok {
					if componentNameInter, ok := obj["component_name"]; ok {
						if componentName, ok := componentNameInter.(string); ok {
							if newName, exists := componentRenames[componentName]; exists {
								obj["component_name"] = newName
							}
						}
					}
				}
			}

			if downstreamInter, ok := newComponent["downstream"]; ok {
				if downstream, ok := downstreamInter.([]interface{}); ok {
					newDownstream := make([]interface{}, len(downstream))
					for i, idInter := range downstream {
						if idStr, ok := idInter.(string); ok {
							if newID, exists := componentIDMap[idStr]; exists {
								newDownstream[i] = newID
							} else {
								newDownstream[i] = idStr
							}
						} else {
							newDownstream[i] = idInter
						}
					}
					newComponent["downstream"] = newDownstream
				}
			}

			if upstreamInter, ok := newComponent["upstream"]; ok {
				if upstream, ok := upstreamInter.([]interface{}); ok {
					newUpstream := make([]interface{}, len(upstream))
					for i, idInter := range upstream {
						if idStr, ok := idInter.(string); ok {
							if newID, exists := componentIDMap[idStr]; exists {
								newUpstream[i] = newID
							} else {
								newUpstream[i] = idStr
							}
						} else {
							newUpstream[i] = idInter
						}
					}
					newComponent["upstream"] = newUpstream
				}
			}

			if parentIDInter, ok := newComponent["parent_id"]; ok {
				if parentID, ok := parentIDInter.(string); ok {
					if newID, exists := componentIDMap[parentID]; exists {
						newComponent["parent_id"] = newID
					}
				}
			}
		}
		rewrittenComponents[newComponentID] = newComponentInter
	}
	normalized["components"] = rewrittenComponents

	if pathInter, ok := normalized["path"]; ok {
		if pathList, ok := pathInter.([]interface{}); ok {
			newPath := make([]interface{}, len(pathList))
			for i, idInter := range pathList {
				if idStr, ok := idInter.(string); ok {
					if newID, exists := componentIDMap[idStr]; exists {
						newPath[i] = newID
					} else {
						newPath[i] = idStr
					}
				} else {
					newPath[i] = idInter
				}
			}
			normalized["path"] = newPath
		}
	}

	if graphInter, ok := normalized["graph"]; ok {
		if graph, ok := graphInter.(map[string]interface{}); ok {
			if nodesInter, ok := graph["nodes"]; ok {
				if nodes, ok := nodesInter.([]interface{}); ok {
					for _, nodeInter := range nodes {
						if node, ok := nodeInter.(map[string]interface{}); ok {
							if nodeIDInter, ok := node["id"]; ok {
								if nodeID, ok := nodeIDInter.(string); ok {
									if newID, exists := componentIDMap[nodeID]; exists {
										node["id"] = newID
									}
								}
							}

							if parentIDInter, ok := node["parentId"]; ok {
								if parentID, ok := parentIDInter.(string); ok {
									if newID, exists := componentIDMap[parentID]; exists {
										node["parentId"] = newID
									}
								}
							}

							if nodeTypeInter, ok := node["type"]; ok {
								if nodeType, ok := nodeTypeInter.(string); ok {
									if newType, exists := nodeTypeRenames[nodeType]; exists {
										node["type"] = newType
									}
								}
							}

							if dataInter, ok := node["data"]; ok {
								if data, ok := dataInter.(map[string]interface{}); ok {
									if labelInter, ok := data["label"]; ok {
										if label, ok := labelInter.(string); ok {
											if newLabel, exists := componentRenames[label]; exists {
												data["label"] = newLabel
											}
										}
									}

									if nameInter, ok := data["name"]; ok {
										if name, ok := nameInter.(string); ok {
											if newName, exists := componentRenames[name]; exists {
												data["name"] = newName
											}
										}
									}

									if formInter, ok := data["form"]; ok {
										data["form"] = rewriteValue(formInter)
									}
								}
							}
						}
					}
				}
			}

			if edgesInter, ok := graph["edges"]; ok {
				if edges, ok := edgesInter.([]interface{}); ok {
					// Prepare replacements sorted by length descending
					type replacement struct {
						oldID string
						newID string
					}
					var replacements []replacement
					for k, v := range componentIDMap {
						replacements = append(replacements, replacement{oldID: k, newID: v})
					}
					sort.Slice(replacements, func(i, j int) bool {
						return len(replacements[i].oldID) > len(replacements[j].oldID)
					})

					for _, edgeInter := range edges {
						if edge, ok := edgeInter.(map[string]interface{}); ok {
							for _, key := range []string{"source", "target"} {
								if valInter, ok := edge[key]; ok {
									if valStr, ok := valInter.(string); ok {
										if newID, exists := componentIDMap[valStr]; exists {
											edge[key] = newID
										}
									}
								}
							}

							if edgeIDInter, ok := edge["id"]; ok {
								if edgeID, ok := edgeIDInter.(string); ok {
									for _, rep := range replacements {
										edgeID = strings.ReplaceAll(edgeID, rep.oldID, rep.newID)
									}
									edge["id"] = edgeID
								}
							}
						}
					}
				}
			}
		}
	}

	for _, key := range []string{"history", "messages", "reference"} {
		if valInter, ok := normalized[key]; ok {
			normalized[key] = rewriteValue(valInter)
		}
	}

	return normalized
}

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

package common

import (
	"sort"
	"strings"
)

// hasComponentIDPrefix reports whether id is "<componentName>:<non-empty instance>".
func hasComponentIDPrefix(id, componentName string) bool {
	kind, instance, ok := strings.Cut(id, ":")
	return ok && kind == componentName && instance != ""
}

// IsExtractorComponentID reports whether id names an Extractor node in a
// component-scoped parser_config or pipeline DSL. Only the canonical
// "Extractor:<instance>" form is accepted.
func IsExtractorComponentID(id string) bool {
	return hasComponentIDPrefix(id, ComponentNameExtractor)
}

// IsExtractorComponent reports whether the component name or component ID
// identifies an Extractor.
func IsExtractorComponent(id, componentName string) bool {
	return componentName == ComponentNameExtractor || IsExtractorComponentID(id)
}

// IsParserComponentID reports whether id names a Parser node in a
// component-scoped parser_config or pipeline DSL.
func IsParserComponentID(id string) bool {
	return hasComponentIDPrefix(id, ComponentNameParser)
}

// IsParserComponent reports whether the component name or component ID
// identifies a Parser.
func IsParserComponent(id, componentName string) bool {
	return componentName == ComponentNameParser || IsParserComponentID(id)
}

// IsCompilerComponentID reports whether id names a Compiler node in a
// component-scoped parser_config or pipeline DSL.
func IsCompilerComponentID(id string) bool {
	return hasComponentIDPrefix(id, ComponentNameCompiler)
}

// IsComponentKey reports whether id is a canonical component-scoped
// parser_config key: "<ComponentName>:<non-empty instance>".
func IsComponentKey(id string) bool {
	kind, instance, ok := strings.Cut(id, ":")
	return ok && instance != "" && IsComponentName(kind)
}

// SortedExtractorKeys returns the parser_config keys that name Extractor nodes,
// sorted so callers resolve multiple Extractor nodes deterministically.
func SortedExtractorKeys(parserConfig map[string]any) []string {
	keys := make([]string, 0)
	for key := range parserConfig {
		if IsExtractorComponentID(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

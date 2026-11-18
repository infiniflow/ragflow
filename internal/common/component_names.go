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

// Canonical ingestion component names.
const (
	ComponentNameFile      = "File"
	ComponentNameParser    = "Parser"
	ComponentNameExtractor = "Extractor"
	ComponentNameCompiler  = "Compiler"
	ComponentNameTokenizer = "Tokenizer"

	ComponentNameGeneralChunker        = "GeneralChunker"
	ComponentNameTokenChunker          = "TokenChunker"
	ComponentNameTitleChunker          = "TitleChunker"
	ComponentNameManualChunker         = "ManualChunker"
	ComponentNameOneChunker            = "OneChunker"
	ComponentNamePageChunker           = "PageChunker"
	ComponentNameQAChunker             = "QAChunker"
	ComponentNameTableChunker          = "TableChunker"
	ComponentNameGroupTitleChunker     = "GroupTitleChunker"
	ComponentNameHierarchyTitleChunker = "HierarchyTitleChunker"
)

// componentNameExtensions holds ingestion component names supplied by an
// edition. The community build registers none, so its canonical name set is
// exactly the list above.
var componentNameExtensions = map[string]struct{}{}

// RegisterComponentName adds an edition component name to the canonical set.
// It must be called from an edition init, before the name is validated.
func RegisterComponentName(name string) {
	if name == "" {
		return
	}
	componentNameExtensions[name] = struct{}{}
}

// IsComponentName reports whether name is a canonical ingestion component name.
func IsComponentName(name string) bool {
	switch name {
	case ComponentNameFile,
		ComponentNameParser,
		ComponentNameExtractor,
		ComponentNameCompiler,
		ComponentNameTokenizer,
		ComponentNameGeneralChunker,
		ComponentNameTokenChunker,
		ComponentNameTitleChunker,
		ComponentNameManualChunker,
		ComponentNameOneChunker,
		ComponentNamePageChunker,
		ComponentNameQAChunker,
		ComponentNameTableChunker,
		ComponentNameGroupTitleChunker,
		ComponentNameHierarchyTitleChunker:
		return true
	default:
		_, ok := componentNameExtensions[name]
		return ok
	}
}

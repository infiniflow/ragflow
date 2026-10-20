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

package schema

// Canonical ingestion component names. These are the identities components
// register under in runtime.DefaultRegistry and the values a DSL
// component_name carries. Layers that must recognise a component by name
// across the component/pipeline boundary (dispatch, param whitelists,
// operator-facing failure messages) reference these constants instead of
// repeating the literals; the schema package is the shared dependency both
// sides already import. More names can join this set as other layers need
// them.
const (
	ComponentNameParser    = "Parser"
	ComponentNameExtractor = "Extractor"
	ComponentNameCompiler  = "Compiler"
)

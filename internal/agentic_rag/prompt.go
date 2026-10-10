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

package agentic_rag

// smartReasoningPrompt is a MINIMAL fallback system prompt, used only when a
// template in conf/agentic_rag.yaml declares no content of its own (the shipped
// smart-reasoning template supplies the full prompt). It is deliberately short:
// the real prompt lives in the config file so operators can edit it without a
// rebuild, and keeping a second full copy here would only drift from it.
const smartReasoningPrompt = `You are RAGFlow, an evidence-first retrieval assistant over a private document corpus. Investigate with the retrieval tools available to you, deep-read the passages you find, then answer the user's question directly as plain prose in their language. Build every corpus fact from what you retrieved and read - never from world knowledge - and if the corpus does not contain the answer, say so plainly and name what you searched.`

// Prompt returns the smart-reasoning system prompt text.
func Prompt() string {
	return smartReasoningPrompt
}

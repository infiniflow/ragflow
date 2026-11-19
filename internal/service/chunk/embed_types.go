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

package chunk

// ChunkEmbedRequest describes the text an embedding turn turns into a vector.
// It carries the knowledgebase's settings rather than reading them, so the
// embedder implementation stays free of the chunk service.
//
// The types live here, outside the `ee`-tagged files, because both editions
// name them in signatures: the enterprise edition embeds edited chunks, the
// community edition answers with a stub that returns none.
type ChunkEmbedRequest struct {
	// TenantID scopes the embedding model resolution.
	TenantID string
	// DatasetID is the knowledgebase whose embedding model is used.
	DatasetID string
	// Name is the document title the tokenizer weights into the vector.
	Name string
	// Text is the chunk content the tokenizer would have embedded.
	Text string
	// Weight is filename_embd_weight for this knowledgebase.
	Weight float64
}

// ChunkEmbedding is the result of an embedding: the engine field the vector is
// stored in (q_<dim>_vec) and the vector itself. An empty Field means the text
// carried nothing to embed.
type ChunkEmbedding struct {
	Field  string
	Vector []float64
}

/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

/**
 * One knowledge-layer lookup.
 *
 * The structure fields are explicit input, never parsed out of the question: an
 * omitted constraint means "no narrowing", which the server grades separately
 * from a narrowing that matched nothing.
 */
export interface IOntologyRetrievalRequestBody {
  /** The knowledge scope: the datasets the compiled rows live in. */
  dataset_ids: string[];
  question: string;
  ontology_template_id?: string;
  /** Zero follows the model's published version. */
  ontology_version?: number;
  document_ids?: string[];
  entity_types?: string[];
  properties?: string[];
  from_types?: string[];
  to_types?: string[];
  top_k?: number;
  /** Absent counts every assertion's quotations as readable. */
  document_scope?: string[];
  skip_unreviewed?: boolean;
}

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
 * The knowledge layer's answer, mirrored from
 * internal/agentic_rag/ontology_service.go.
 *
 * The grade is stated by the server rather than derived here: the panel shows
 * what the layer concluded, and a UI that recomputed "was this narrowed" from
 * the presence of a filter would be a second opinion about the same rows.
 */
export const OntologyEvidenceLevel = {
  /** Narrowed by the declared structure and matched. */
  Structured: 'structured',
  /** Compiled rows retrieved without a declared narrowing. */
  SchemaFree: 'schema_free',
  /** Nothing came back from either stage. */
  None: 'none',
} as const;

export type OntologyEvidenceLevelValue =
  (typeof OntologyEvidenceLevel)[keyof typeof OntologyEvidenceLevel];

export interface IOntologyModelRef {
  id: string;
  name: string;
  version: number;
  publish_status: string;
}

/**
 * One compiled row.
 *
 * Everything but the role is optional because the two surfaces that carry a row
 * agree on the row and not on how much of it they ship: the preview answers with
 * the whole digest, an answer's receipt carries the endpoints and the quotation
 * and leaves the rest out.
 */
export interface IOntologyAssertion {
  /** `entity` or `relation`: the row's compiled role. */
  kind: string;
  /** The declared class, or the property for a relation. */
  type?: string;
  name?: string;
  from?: string;
  from_type?: string;
  to?: string;
  to_type?: string;
  attributes?: Record<string, string> | null;
  /** The verbatim quotation the compiler kept, not a re-worded claim. */
  evidence?: string;
  chunk_ids?: string[] | null;
  doc_ids?: string[] | null;
  review_status?: string;
  review_by?: string;
}

export interface IOntologyCoverage {
  declared_classes: string[] | null;
  declared_properties: string[] | null;
  matched_classes: string[] | null;
  matched_properties: string[] | null;
  missing_classes: string[] | null;
  missing_properties: string[] | null;
  sample_limit: number;
  sampled: number;
  truncated: boolean;
}

/** Assertions whose source chunks resolve in the reading scope, and those that do not. */
export interface IOntologyCitations {
  resolvable: number;
  unresolvable: number;
}

export interface IOntologyDegrade {
  code: string;
  reason: string;
}

export interface IOntologyDigest {
  model: IOntologyModelRef;
  assertions: IOntologyAssertion[];
  vocabulary: string[] | null;
  coverage: IOntologyCoverage;
  citations: IOntologyCitations;
  applied_filter: Record<string, unknown> | null;
  structure_applied: boolean;
  /** One of OntologyEvidenceLevel; a value this build does not know is shown as-is. */
  evidence_level: string;
  degrade: IOntologyDegrade | null;
  unreviewed_count: number;
  total: number;
}

export interface IOntologyConstraints {
  entity_types?: string[] | null;
  properties?: string[] | null;
  from_types?: string[] | null;
  to_types?: string[] | null;
}

/**
 * The envelope the endpoint answers with.
 *
 * `chunks` is the same rows the digest is projected from, kept because a caller
 * may want the engine's own shape; the panel reads the digest.
 */
/**
 * The retrieval receipt that travels with an answer, under
 * `reference.ontology`.
 *
 * It is the same facts as the digest above, flattened for the wire and spelled
 * by one server-side renderer, so what a person reads beside an answer and what
 * they saw in the preview describe the same retrieval the same way. Counts are
 * always present, including the zeroes: a citation count that disappeared when
 * it was zero could not be told apart from a receipt that never computed it.
 */
export interface IOntologyReceipt {
  model: IOntologyModelRef;
  vocabulary: string[] | null;
  applied_filter: Record<string, unknown> | null;
  structure_applied: boolean;
  evidence_level: string;
  unreviewed_count: number;
  /** How many rows this answer carried, against rows_total the engine reported. */
  rows_carried: number;
  rows_total: number;
  citations: IOntologyCitations;
  coverage: IOntologyReceiptCoverage;
  degrade?: IOntologyDegrade | null;
  /** Present only when the turn carried rows. */
  assertions?: IOntologyAssertion[] | null;
}

export interface IOntologyReceiptCoverage {
  declared_classes: string[] | null;
  declared_properties: string[] | null;
  matched_classes: string[] | null;
  matched_properties: string[] | null;
  missing_classes: string[] | null;
  missing_properties: string[] | null;
  sampled: number;
  sample_limit: number;
  sample_truncated: boolean;
}

export interface IOntologyRetrievalResult {
  chunks: Record<string, unknown>[] | null;
  total: number;
  filter: Record<string, unknown> | null;
  evidence_level: string;
  structure_applied: boolean;
  degrade_code: string;
  degrade_reason: string;
  model: IOntologyModelRef;
  suggested_constraints: IOntologyConstraints | null;
  digest: IOntologyDigest | null;
}

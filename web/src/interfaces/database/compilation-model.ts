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

export enum CompilationModelPublishStatus {
  Draft = 'draft',
  Review = 'review',
  Published = 'published',
  Retired = 'retired',
}

export enum CompilationModelIssueSeverity {
  Blocker = 'blocker',
  Warning = 'warning',
}

/**
 * One finding from the pre-publish check.
 *
 * The blocker / warning split is the mechanism of the gate: a warning is a
 * judgement the publisher may overrule, a blocker is not.
 */
export interface ICompilationModelIssue {
  code: string;
  category?: string;
  severity: CompilationModelIssueSeverity | string;
  message: string;
  subjects?: string[];
}

/** The report the publish gate decides on. Returned on refusals too. */
export interface ICompilationModelPublishCheck {
  template_id: string;
  name: string;
  kind: string;
  current_version: number;
  next_version: number;
  blockers: ICompilationModelIssue[];
  warnings: ICompilationModelIssue[];
  publishable: boolean;
}

export interface ICompilationModel {
  id: string;
  name: string;
  kind: string;
  group_id?: string;
  description?: string;
  /** Published version number; 0 means never published. */
  version: number;
  publish_status: CompilationModelPublishStatus | string;
  published_at?: string;
  published_by?: string;
  updated_at?: string;
  review_note?: string;
  /**
   * True when the working config differs from the published version, so the
   * model on screen is not the one the corpus was extracted with.
   */
  unpublished_changes: boolean;
  /** How many datasets compile against this model. */
  dataset_count: number;
}

/** One dataset a change to a model reaches. */
export interface ICompilationModelImpactDataset {
  id: string;
  name: string;
  doc_num: number;
  update_time?: number;
}

/**
 * The drill-down behind the dataset count.
 *
 * The count on the listing is a prompt — it says a change reaches something.
 * This is the decision: a publisher told "3 datasets" has to know which three
 * to tell whether the change can be made now or has to be scheduled.
 */
export interface ICompilationModelImpact {
  template_id: string;
  /** The reference datasets store: the model's group, or the template itself. */
  ref: { kind: string; id: string };
  datasets: ICompilationModelImpactDataset[];
}

/** One piece of knowledge a compile run produced. */
export interface ICompilationModelTrialAssertion {
  kind: string;
  type?: string;
  name: string;
  key: string;
}

/** One document's rehearsal result. */
export interface ICompilationModelTrialDocument {
  dataset_id: string;
  dataset_name: string;
  document_id: string;
  document_name: string;
  chunks: number;
  added: ICompilationModelTrialAssertion[];
  removed: ICompilationModelTrialAssertion[];
  unchanged: number;
  /** Why this document produced no comparison. */
  error?: string;
}

/**
 * The pre-publish rehearsal: the draft model run over real documents, compared
 * with what the corpus holds under the published version.
 *
 * Separate from the publish check because it is not free — it calls the model —
 * and because it answers a different question: the check asks whether the model
 * is self-consistent, the rehearsal asks what it would change.
 */
export interface ICompilationModelTrial {
  template_id: string;
  kind: string;
  current_version: number;
  draft_version: number;
  documents: ICompilationModelTrialDocument[];
  added: number;
  removed: number;
  runs: number;
  truncated: boolean;
  notes: string[];
}

/**
 * One compiled instance or assertion, as it sits in the corpus.
 *
 * `dataset_id` / `document_ids` / `source_chunk_ids` are what make a row
 * inspectable: an instance whose source text cannot be opened cannot be judged,
 * and a model is compiled by many datasets, so the row also has to say which one
 * it came from.
 */
/**
 * One verbatim quote supporting an assertion, and the chunk it was read from.
 *
 * The span is what the compiler located, in BYTE offsets, because that is what
 * the gate can align on. The panel does not slice by it: a JS string index is a
 * UTF-16 code unit, so a byte offset lands in the wrong place for any text with
 * CJK or accented characters. It locates the quote by text and uses the span
 * only as a hint.
 */
export interface IOntologyEvidence {
  quote: string;
  chunk_id?: string;
  start?: number;
  end?: number;
}

export interface ICompilationModelInstance {
  name?: string;
  type?: string;
  description?: string;
  aliases?: string[];
  mention_count?: number;
  from?: string;
  to?: string;
  source_chunk_ids?: string[];
  document_ids?: string[];
  /** The quotes this assertion was read from, when the template asked for them. */
  evidence?: IOntologyEvidence[];
  dataset_id?: string;
  dataset_name?: string;
  /**
   * The published model version that extracted this row, when it recorded one.
   * Lower than the model's current version means it has not been recompiled
   * since — the row is what an earlier model said, not what this one says.
   */
  version?: number;
  /** The row's identity inside its dataset; a review has to name the row it judges. */
  id?: string;
  /** How far the assertion reaches: one document, or every document of the dataset. */
  scope?: AssertionScope;
  review_status?: AssertionReviewVerdict;
  review_by?: string;
  /** Epoch milliseconds, as the row records it. */
  review_at?: number;
  review_note?: string;
  /** True when another row of the same class and endpoint disagrees with this one. */
  conflict?: boolean;
  conflict_targets?: string[];
  conflict_types?: string[];
}

/** One dataset the inspection read. */
export interface ICompilationModelInspectDataset {
  id: string;
  name: string;
  doc_num: number;
}

/** How wide an inspection reads. */
export enum CompilationModelInspectScope {
  Dataset = 'dataset',
  Corpus = 'corpus',
}

export interface ICompilationModelInspectParams {
  /**
   * Which row kind to read: the panel's two tabs are two kinds, not two
   * filters, so the tab says it and class/property only narrow within it.
   */
  kind?: AssertionKind;
  /** Omitted means every class of the model, not "nothing". */
  class?: string;
  /** Omitted means every property of the model, not "nothing". */
  property?: string;
  source_type?: string;
  target_type?: string;
  scope?: CompilationModelInspectScope | string;
  dataset_ids?: string;
  limit?: number;
}

export interface ICompilationModelInspect {
  template_id: string;
  scope: string;
  /**
   * Which of the two row lists this response answers. It is reported because an
   * unfiltered read names neither a class nor a property, so nothing else would
   * say which list was populated.
   */
  kind: AssertionKind;
  class?: string;
  property?: string;
  source?: string;
  target?: string;
  /** The version in force, to compare each row's own version against. */
  model_version: number;
  datasets: ICompilationModelInspectDataset[];
  /** Datasets the read cap left out: "did not look" must not read as "empty". */
  skipped_dataset_ids?: string[];
  total: number;
  truncated: boolean;
  entities: ICompilationModelInstance[];
  relations: ICompilationModelInstance[];
}

/**
 * What a row asserts, and how far it reaches.
 *
 * The two are separate because they change what a conflict means: a class row
 * and a property row can only conflict with their own kind, and a doc-scope row
 * disagrees with a dataset-scope one by being narrower, not by being wrong.
 */
export type AssertionKind = 'entity' | 'relation';

export type AssertionScope = 'doc' | 'dataset';

/**
 * The vocabulary a queue is read with, plus the two states a judged row can be in.
 *
 * `unreviewed` is not a state a row carries: absence of a review is how the
 * corpus says "not judged yet", so nothing is written when a row is compiled.
 * `all` exists because a queue that could only show one verdict at a time would
 * not answer "what has been settled so far", which is the question a reviewer
 * asks when deciding whether the corpus can be trusted.
 */
export type AssertionReviewStatus =
  | 'unreviewed'
  | 'accepted'
  | 'rejected'
  | 'all';

/** The state a judged row carries. */
export type AssertionReviewVerdict = Exclude<
  AssertionReviewStatus,
  'unreviewed' | 'all'
>;

/** What a review does to a row. `reset` takes a verdict back to unreviewed. */
export type AssertionReviewAction = 'accept' | 'reject' | 'reset';

export interface IAssertionReviewCounts {
  unreviewed: number;
  accepted: number;
  rejected: number;
}

/**
 * One assertion waiting to be judged, with what judging it needs.
 *
 * It carries the whole row rather than an id because the decision is made from
 * the row and its source text: a reviewer shown a bare id would have to open a
 * second panel to find out what they are accepting.
 */
export interface IAssertionReviewItem {
  id: string;
  dataset_id: string;
  dataset_name?: string;
  kind: AssertionKind;
  scope: AssertionScope;
  name?: string;
  type?: string;
  description?: string;
  mention_count?: number;
  from?: string;
  to?: string;
  property?: string;
  source_type?: string;
  target_type?: string;
  version?: number;
  source_chunk_ids?: string[];
  document_ids?: string[];
  evidence?: IOntologyEvidence[];
  /** Absent means unreviewed — the corpus keeps no row for "not judged yet". */
  review_status?: AssertionReviewVerdict;
  review_by?: string;
  /** Epoch milliseconds, like every other timestamp the API returns. */
  review_at?: number;
  review_note?: string;
  conflict?: boolean;
  /** The rows this one disagrees with, by name. */
  conflict_targets?: string[];
  /** What the disagreement is about, e.g. a class admitted by one side only. */
  conflict_types?: string[];
}

/**
 * One page of the review queue.
 *
 * `counts` covers the whole queue while `items` is a window of it, so the
 * progress line stays honest at any page: the reader has to know how much is
 * left, not how much is on screen.
 */
export interface IAssertionReviewQueue {
  template_id: string;
  model_version: number;
  datasets: ICompilationModelInspectDataset[];
  /** Datasets the read cap left out: "did not look" must not read as "empty". */
  skipped_dataset_ids: string[];
  counts: IAssertionReviewCounts;
  total: number;
  truncated: boolean;
  conflicts: number;
  items: IAssertionReviewItem[];
}

/**
 * One row a review could not land on.
 *
 * A batch is applied per row, so a partial result is normal and the reasons
 * have to come back with the ids: a reviewer keeps the failed rows selected and
 * retries them without guessing which ones they were.
 */
export interface IAssertionReviewFailure {
  id: string;
  reason: string;
}

export interface IAssertionReviewResult {
  updated: number;
  failed: IAssertionReviewFailure[];
  counts: IAssertionReviewCounts;
}

export interface ICompilationModelVersion {
  version: number;
  name: string;
  note?: string;
  published_by?: string;
  published_at?: string;
  is_current: boolean;
}

export interface ICompilationModelVersionChange {
  /** JSON path, e.g. `entity.fields[place].parent`. */
  path: string;
  change: 'added' | 'removed' | 'modified';
  before?: string;
  after?: string;
}

export interface ICompilationModelVersionDiff {
  template_id: string;
  from: number;
  to: number;
  changes: ICompilationModelVersionChange[];
}

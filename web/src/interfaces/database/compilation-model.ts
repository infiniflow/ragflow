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
  dataset_id?: string;
  dataset_name?: string;
  /**
   * The published model version that extracted this row, when it recorded one.
   * Lower than the model's current version means it has not been recompiled
   * since — the row is what an earlier model said, not what this one says.
   */
  version?: number;
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
  class?: string;
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

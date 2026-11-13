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
  /**
   * Contract coverage over actions only. An action with no authorization is the
   * row that decides whether the action can ever be automated, so it is counted
   * apart from the generic unreviewed number.
   */
  contract_missing_authorization?: number;
  contract_missing_precondition?: number;
  /** Unreviewed actions that DO carry an authorization: a bulk accept must skip them. */
  contract_unreviewed_critical?: number;
}

/** The three-state judgement of one contract field. */
export type ActionContractFieldStatus = 'present' | 'missing' | 'explicit_none';

/**
 * Where a field's value came from.
 *
 * `manual` may not be quoted or cited: an operator entered it, no document says
 * it. The UI has to show the difference, because a value that looks the same on
 * screen means something different in an answer.
 */
export type ActionContractFieldProvenance = 'document' | 'manual' | 'absent';

export type ActionContractFieldName =
  | 'authorization'
  | 'preconditions'
  | 'atomic_effects'
  | 'follow_ups'
  | 'failure_policy'
  | 'audit_required';

export interface IActionContractField {
  status: ActionContractFieldStatus;
  value?: unknown;
  /** The verbatim sentence the value was read from. */
  quote?: string;
  provenance: ActionContractFieldProvenance;
}

export interface IActionContractView {
  fields: Record<ActionContractFieldName, IActionContractField>;
  missing: ActionContractFieldName[];
  /**
   * The declaration this contract is judged against.
   *
   * A template may narrow the contract to the fields its actions actually need,
   * so this is the denominator of `declared`: a contract asked for two fields and
   * filled both is complete, not "2/6". `fields` still carries all six — a value
   * stored before the template narrowed is real and must stay editable.
   */
  required: ActionContractFieldName[];
  declared: number;
}

/** Contract field order, matching the server. */
export const ACTION_CONTRACT_FIELD_ORDER: ActionContractFieldName[] = [
  'authorization',
  'preconditions',
  'atomic_effects',
  'follow_ups',
  'failure_policy',
  'audit_required',
];

/**
 * The four verdicts, in the order they escalate.
 *
 * `confirm` is not a weaker `allow`: it means a human must approve, which is why
 * the UI renders it as its own state instead of as a soft warning.
 */
export type ActionVerdict = 'allow' | 'confirm' | 'manual' | 'refuse';

/** One action with its contract, binding and the verdict computed from both. */
export interface IModelAction {
  id: string;
  name: string;
  dataset_id: string;
  description?: string;
  contract: IActionContractView;
  verdict: ActionVerdict;
  /** Why the verdict is what it is. Empty for allow. */
  verdict_reasons?: string[];
  binding?: IActionBinding;
  review_status?: string;
}

/**
 * An action's link to the MCP tool that executes it.
 *
 * `tool_available` is computed server-side on every read: a tool can disappear
 * from its server without any write here, so this cannot be a stored flag.
 */
export interface IActionBinding {
  id: string;
  action_name: string;
  dataset_id?: string;
  mcp_server: string;
  mcp_tool: string;
  arg_mapping?: Record<string, string>;
  /** Declared, not assumed: an action is not read-only until someone says so. */
  read_only: boolean;
  enabled: boolean;
  /** Whether an agent turn may call it unattended. */
  qa_callable: boolean;
  note?: string;
  tool_available: boolean;
  tool_check_error?: string;
  missing_required_args?: string[];
  verdict: ActionVerdict;
  verdict_reasons?: string[];
}

/** The outcome of a dry run or an execution. */
export interface IActionCallResult {
  action_name: string;
  verdict: ActionVerdict;
  reasons?: string[];
  status: string;
  output?: string;
  error?: string;
  duration_ms?: number;
  /** The exact arguments the mapping produced — what makes a dry run worth reading. */
  arguments?: Record<string, unknown>;
}

/** One attempt, including the ones that were refused. */
export interface IActionExecution {
  id: string;
  action_name: string;
  mode: 'query' | 'manual' | string;
  mcp_tool: string;
  verdict: string;
  /** Which gates decided it. Empty on a clean allow. */
  verdict_reasons?: string[];
  status: string;
  request_arg?: Record<string, unknown>;
  response_excerpt?: string;
  error?: string;
  duration_ms: number;
  operator_id?: string;
  confirmed_by?: string;
  create_time?: number;
}

export interface IActionExecutionList {
  executions: IActionExecution[];
  total: number;
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
  /**
   * The judged six-field contract, present only for actions. It is the server's
   * judgement (status + provenance), not the raw column: "what counts as filled"
   * is a safety decision, and a second implementation here would be a second
   * answer to it.
   */
  contract_fields?: Record<string, IActionContractField>;
  /** The required fields without a value, judged against `contract_required`. */
  contract_missing?: string[];
  /** The declaration the contract was judged against: the fields actually owed. */
  contract_required?: ActionContractFieldName[];
  /** The rows this one disagrees with, by name. */
  conflict_targets?: string[];
  /** What the disagreement is about, e.g. a class admitted by one side only. */
  conflict_types?: string[];
}

/**
 * One dataset a cross-dataset read touched, as the review queue reports it.
 *
 * The rows themselves are browsed per dataset; this list exists so a queue that
 * could not read everything can say which datasets it skipped.
 */
export interface ICompilationModelInspectDataset {
  id: string;
  name: string;
  doc_num: number;
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

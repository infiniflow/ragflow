export type BusinessDocumentLifecycleState =
  | 'INTAKE'
  | 'REVIEW'
  | 'AGREED'
  | 'ARCHIVED';

export type BusinessDocumentOperationState =
  | 'IDLE'
  | 'ANALYZING'
  | 'ANALYZING_REVIEW'
  | 'GENERATING_DRAFT'
  | 'APPLYING_CHANGES'
  | 'EXPORTING'
  | 'FAILED';

export type BusinessDocumentCommandType =
  | 'REQUEST_INTAKE_ASSESSMENT'
  | 'REQUEST_REVIEW_ASSESSMENT'
  | 'ANSWER_QUESTION'
  | 'REQUEST_DRAFT'
  | 'DECIDE_PROPOSAL'
  | 'ADD_COMMENT'
  | 'APPLY_CHANGES'
  | 'START_REVIEW'
  | 'REQUEST_EXPORT'
  | 'ARCHIVE';

export type BusinessDocumentBlock =
  | { type: 'paragraph'; text: string }
  | { type: 'list'; items: string[] }
  | {
      type: 'table';
      headers: string[];
      rows: Array<Array<string | number | boolean | null>>;
    }
  | { type: 'plantuml'; source: string }
  | { type: 'image'; alt: string; url: string }
  | { type: 'reference'; label: string; url: string };

export interface BusinessDocumentSection {
  id: string;
  title: string;
  blocks: BusinessDocumentBlock[];
  evidence_refs?: string[];
}

export interface BusinessDocumentAst {
  schema_version: '1';
  document_type: 'business_requirements';
  template_version: string;
  sections: BusinessDocumentSection[];
}

export interface BusinessDocumentRevision {
  revision_id: string;
  revision_number: number;
  author_id?: string | null;
  author_name?: string | null;
  document_ast: BusinessDocumentAst;
  section_texts: Record<string, string>;
  body_markdown: string;
  content_hash: string;
  source_event_ids?: string[];
  created_at?: number | null;
  change_basis?: BusinessDocumentRevisionBasis[];
}

export interface BusinessDocumentRevisionBasis {
  event_id: string;
  type: 'INITIAL_DRAFT' | 'QUESTION' | 'PROPOSAL' | 'COMMENT' | 'EVA_SYNC';
  title: string;
  summary: string;
  details?: string | null;
  section_id?: string | null;
  actor_id?: string;
  actor_type?: 'USER' | 'AI' | 'SYSTEM';
  initiated_by_actor_id?: string | null;
  created_at?: number | null;
}

export type BusinessDocumentEvaCapability =
  | 'OPEN'
  | 'PULL_FROM_EVA'
  | 'CREATE_EVA_CHANGE';

export interface BusinessDocumentEvaBinding {
  page_url: string;
  status: 'LINK_ONLY' | 'CONNECTED';
  capabilities: BusinessDocumentEvaCapability[];
  connector_id?: string | null;
  project_id?: string | null;
  document_id?: string | null;
  document_code?: string | null;
  document_name?: string | null;
  remote_version?: string | null;
  remote_content_hash?: string | null;
  last_pulled_content_hash?: string | null;
  last_pulled_at?: number | null;
  last_pull_event_id?: string | null;
  last_pull_review_cycle?: number | null;
}

export interface BusinessDocumentSelection {
  revision_id: string;
  section_id: string;
  selected_text: string;
  prefix: string;
  suffix: string;
  start_offset: number;
  end_offset: number;
}

export interface BusinessDocumentExportArtifact {
  artifact_id: string;
  revision_id: string;
  revision_number: number | null;
  format: 'MARKDOWN' | 'DOCX' | 'EVA_WIKI';
  filename: string;
  mime_type: string;
  size: number;
  content_hash: string;
  create_time: number | null;
}

export interface BusinessDocumentQuestionOption {
  option_id: string;
  label: string;
  description?: string;
}

export interface BusinessDocumentQuestion {
  question_id: string;
  sequence_number?: number;
  target_section_id?: string;
  text: string;
  options: BusinessDocumentQuestionOption[];
  allow_custom_answer: boolean;
  status: 'OPEN' | 'ANSWERED' | 'CANCELLED';
  answer?: {
    answer_id?: string;
    selected_option_id?: string | null;
    custom_answer?: string | null;
    actor_id?: string;
  };
}

export interface BusinessDocumentProposal {
  proposal_id: string;
  sequence_number?: number;
  target_section_id?: string;
  text: string;
  rationale?: string;
  decision: 'PENDING' | 'ACCEPTED' | 'REJECTED';
}

export interface BusinessDocumentCommentAnchor {
  revision_id: string;
  section_id: string;
  selected_text: string;
  prefix: string;
  suffix: string;
  start_offset: number;
  end_offset: number;
}

export interface BusinessDocumentCommentDisposition {
  comment_event_id: string;
  disposition: 'CONFIRMED_CHANGE' | 'NEEDS_QUESTION' | 'NO_CHANGE';
  question_id?: string;
  question_semantic_tag?: string;
}

export interface BusinessDocumentComment {
  comment_id: string;
  text: string;
  anchor?: BusinessDocumentCommentAnchor | null;
  anchor_status: 'GENERAL' | 'ANCHORED' | 'ORPHANED';
  revision_id?: string;
  section_id?: string | null;
  disposition?: BusinessDocumentCommentDisposition | null;
}

export interface BusinessDocumentReviewCycle {
  questions: BusinessDocumentQuestion[];
  proposals: BusinessDocumentProposal[];
  comments: BusinessDocumentComment[];
}

export interface BusinessDocumentJobSummary {
  job_id: string;
  job_type: string;
  status: 'PENDING' | 'RUNNING' | 'RETRY' | 'COMPLETED' | 'DEAD';
  progress?: number;
  progress_stage?: string;
  progress_message?: string | null;
  attempt: number;
  max_attempts: number;
  available_at?: number | null;
  lease_expires_at?: number | null;
  error?:
    | { code?: string; message?: string; details?: unknown }
    | string
    | null;
  create_time?: number | null;
  update_time?: number | null;
}

export interface BusinessDocumentProjection {
  document_id: string;
  owner_id?: string;
  owner_name?: string | null;
  access_role?: BusinessDocumentRole;
  permissions?: BusinessDocumentPermissions;
  title: string;
  document_type?: string;
  idea?: string;
  dataset_ids?: string[];
  state_version: number;
  lifecycle_state: BusinessDocumentLifecycleState;
  operation_state: BusinessDocumentOperationState;
  current_revision: BusinessDocumentRevision | null;
  active_review_cycle: number;
  protocol: BusinessDocumentReviewCycle;
  allowed_commands: BusinessDocumentCommandType[];
  last_error?: { code?: string; message?: string } | string | null;
  latest_job?: BusinessDocumentJobSummary | null;
  latest_exports?: BusinessDocumentExportArtifact[];
  eva_binding?: BusinessDocumentEvaBinding | null;
}

export interface BusinessDocumentSummary {
  document_id: string;
  owner_id?: string;
  owner_name?: string | null;
  access_role?: BusinessDocumentRole;
  permissions?: BusinessDocumentPermissions;
  title: string;
  lifecycle_state: BusinessDocumentLifecycleState;
  operation_state: BusinessDocumentOperationState;
  state_version: number;
  current_revision_number: number | null;
  eva_page_url?: string | null;
  latest_job?: BusinessDocumentJobSummary | null;
  update_time: number | null;
}

export interface BusinessDocumentPermissions {
  read: boolean;
  edit: boolean;
  delete: boolean;
  assign?: boolean;
}

export type BusinessDocumentRole =
  | 'AUTHOR_CREATOR'
  | 'AUTHOR_EDITOR'
  | 'MODERATOR_CREATOR'
  | 'EXTENDED_MODERATOR'
  | 'ADMIN';

export interface BusinessDocumentCapabilities {
  read: boolean;
  create: boolean;
  edit_own: boolean;
  edit_all: boolean;
  delete: boolean;
  assign: boolean;
}

export interface BusinessDocumentAssignableUser {
  user_id: string;
  nickname: string;
  email: string;
  role: BusinessDocumentRole;
}

export interface DeleteBusinessDocumentResult {
  document_id: string;
  deleted: true;
  deleted_artifacts: number;
  storage_cleanup_failures: number;
}

export interface BusinessDocumentList {
  items: BusinessDocumentSummary[];
  total: number;
  page: number;
  page_size: number;
  scope?: 'mine' | 'all';
  access_role?: BusinessDocumentRole;
  capabilities?: BusinessDocumentCapabilities;
}

export interface CreateBusinessDocumentRequest {
  schema_version: '1';
  document_type: 'business_requirements';
  title: string;
  idea: string;
  dataset_ids?: string[];
  eva_page_url?: string;
}

export interface BusinessDocumentEvaPullResult {
  document: BusinessDocumentProjection;
  sync: {
    changed: boolean;
    direction: 'FROM_EVA';
    event_id?: string;
    remote_version?: string | null;
  };
}

export interface BusinessDocumentCommand {
  schema_version: '1';
  command_id: string;
  idempotency_key: string;
  expected_state_version: number;
  type: BusinessDocumentCommandType;
  payload: Record<string, unknown>;
}

export interface BusinessDocumentCommandResult {
  accepted: boolean;
  document_id: string;
  state_version: number;
  lifecycle_state: BusinessDocumentLifecycleState;
  operation_state: BusinessDocumentOperationState;
  job_id?: string | null;
  event_id?: string;
  allowed_commands?: BusinessDocumentCommandType[];
  idempotent_replay?: boolean;
}

export type EvaDocumentChangeState =
  | 'EDITING'
  | 'APPROVED'
  | 'PREPARING_EVA_DRAFT'
  | 'EVA_DRAFT_READY'
  | 'PUBLISHING'
  | 'PUBLISHED';

export type EvaDocumentChangeAction =
  | 'SAVE_DRAFT'
  | 'APPROVE'
  | 'PREPARE_EVA_DRAFT'
  | 'PUBLISH_EVA';

export interface EvaDocumentSource {
  connector_id: string;
  connector_name: string;
  id: string;
  name: string;
  code: string;
  project_id: string;
  version: string;
  modified_at: string;
  web_url: string;
  excerpt: string;
}

export interface EvaDocumentSourceSearchResult {
  items: EvaDocumentSource[];
  connectors: Array<{ connector_id: string; connector_name: string }>;
}

export interface EvaDocumentDiffLine {
  type: 'context' | 'added' | 'removed';
  content: string;
}

export interface EvaDocumentSectionDiff {
  key: string;
  title: string;
  lines: EvaDocumentDiffLine[];
}

export interface EvaDocumentDiff {
  changed: boolean;
  added_lines: number;
  removed_lines: number;
  changed_sections: number;
  sections: EvaDocumentSectionDiff[];
}

export interface EvaDocumentChangeEvent {
  event_id: string;
  sequence: number;
  event_type: string;
  actor_id: string;
  payload: Record<string, unknown>;
  create_time: number | null;
}

export interface EvaDocumentChange {
  change_id: string;
  state_version: number;
  workflow_state: EvaDocumentChangeState;
  change_summary: string;
  source: {
    connector_id: string;
    project_id: string;
    document_id: string;
    document_code?: string | null;
    document_name: string;
    web_url?: string | null;
    base_version: string;
    base_content_hash: string;
  };
  base_markdown: string;
  draft_markdown: string;
  draft_content_hash: string;
  diff: EvaDocumentDiff;
  allowed_actions: EvaDocumentChangeAction[];
  approved_at?: number | null;
  eva_draft_at?: number | null;
  published_at?: number | null;
  published_version?: string | null;
  last_error?: { code?: string; message?: string; details?: unknown } | null;
  operation_retry_after_ms?: number | null;
  events: EvaDocumentChangeEvent[];
}

export interface EvaDocumentChangeSummary {
  change_id: string;
  document_name: string;
  document_code?: string | null;
  change_summary: string;
  workflow_state: EvaDocumentChangeState;
  state_version: number;
  update_time: number | null;
  web_url?: string | null;
}

export interface EvaDocumentChangeList {
  items: EvaDocumentChangeSummary[];
  total: number;
  page: number;
  page_size: number;
}

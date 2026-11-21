export interface IFetchCompilationTemplatesRequestParams {
  keywords?: string;
  page?: number;
  page_size?: number;
  kind?: string;
}

export interface ICompilationTemplateSectionRequest {
  description?: string;
  fields: Array<Record<string, string>>;
}

export interface ICompilationTemplateRaptorConfigRequest {
  prompt?: string;
  max_token?: number;
  clustering_threshold?: number;
  clustering_ratio?: number;
  rechunk?: boolean;
  claim_prompt?: string;
}

export interface ICompilationTemplateConfigRequest {
  kind?: string;
  llm_id?: string;
  mode?: 'entity' | 'topic';
  entity?: ICompilationTemplateSectionRequest;
  relation?: ICompilationTemplateSectionRequest;
  raptor?: ICompilationTemplateRaptorConfigRequest;
  global_rules?: string;
  rechunk?: boolean;
  rechunk_rules?: string;
  [section: string]:
    | ICompilationTemplateSectionRequest
    | ICompilationTemplateRaptorConfigRequest
    | Record<string, unknown>
    | string
    | boolean
    | undefined;
}

export interface ICreateCompilationTemplateRequestBody {
  name: string;
  description?: string;
  kind: string;
  config: ICompilationTemplateConfigRequest;
}

export type IUpdateCompilationTemplateRequestBody =
  Partial<ICreateCompilationTemplateRequestBody>;

/**
 * One edit the compile-quality ledger can apply to an ontology template. The
 * vocabulary is closed on purpose: the panel proposes one of these, the service
 * decides whether the template can take it.
 */
export interface IOntologyFixRequestBody {
  /** "widen" admits a class into an existing declaration; "declare" adds one. */
  op: 'widen' | 'declare';
  property: string;
  /** Which endpoint a widened declaration changes. */
  side?: 'domain' | 'range';
  /** The class a widened declaration must admit. */
  class?: string;
  domain?: string;
  range?: string;
  datatype?: string;
}

/**
 * What an ontology edit comes back as. The service unwraps the axios response,
 * so this is the envelope itself — the same shape every other call in this file
 * reaches through `.data`.
 */
export interface IOntologyFixResponse {
  code: number;
  message?: string;
}

/**
 * A batch of ledger edits, applied as one write. The batch exists because the
 * expensive step is the re-compile that follows it, not the edit itself: a
 * reader selects several lines and compiles once.
 */
export interface IOntologyFixBatchRequestBody {
  fixes: IOntologyFixRequestBody[];
}

export interface ICreateCompilationTemplateGroupRequestBody {
  name: string;
  description?: string;
  avatar?: string;
  templates: Array<{
    id?: string;
    name?: string;
    description?: string;
    kind: string;
    config: ICompilationTemplateConfigRequest;
  }>;
}

export type IUpdateCompilationTemplateGroupRequestBody =
  Partial<ICreateCompilationTemplateGroupRequestBody>;

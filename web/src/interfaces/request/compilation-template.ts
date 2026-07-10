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
  threshold?: number;
  rechunk?: boolean;
}

export interface ICompilationTemplateConfigRequest {
  kind?: string;
  llm_id?: string;
  entity?: ICompilationTemplateSectionRequest;
  relation?: ICompilationTemplateSectionRequest;
  raptor?: ICompilationTemplateRaptorConfigRequest;
  global_rules?: string;
  [section: string]:
    | ICompilationTemplateSectionRequest
    | ICompilationTemplateRaptorConfigRequest
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

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

export interface ICompilationTemplateConfigRequest {
  kind?: string;
  llm_id?: string;
  entity?: ICompilationTemplateSectionRequest;
  relation?: ICompilationTemplateSectionRequest;
  global_rules?: string;
  [section: string]: ICompilationTemplateSectionRequest | string | undefined;
}

export interface ICreateCompilationTemplateRequestBody {
  name: string;
  description?: string;
  kind: string;
  config: ICompilationTemplateConfigRequest;
}

export type IUpdateCompilationTemplateRequestBody =
  Partial<ICreateCompilationTemplateRequestBody>;

export interface ICompilationTemplateField {
  type?: string;
  description?: string;
  rule?: string;
  [key: string]: string | undefined;
}

export interface ICompilationTemplateSection {
  description?: string;
  fields: ICompilationTemplateField[];
}

export interface ICompilationTemplateRaptorConfig {
  prompt?: string;
  max_token?: number;
  threshold?: number;
  rechunk?: boolean;
}

export interface ICompilationTemplateConfig {
  kind?: string;
  llm_id?: string;
  entity?: ICompilationTemplateSection;
  relation?: ICompilationTemplateSection;
  raptor?: ICompilationTemplateRaptorConfig;
  global_rules?: string;
  [section: string]:
    | ICompilationTemplateSection
    | ICompilationTemplateRaptorConfig
    | Record<string, unknown>
    | string
    | boolean
    | undefined;
}

export interface ICompilationTemplate {
  id: string;
  name: string;
  description: string;
  kind: string;
  config: ICompilationTemplateConfig;
  create_time?: number;
  update_time?: number;
}

export interface ICompilationTemplateListResult {
  templates: ICompilationTemplate[];
  total: number;
}

export interface ICompilationTemplateBuiltin {
  id: string;
  kind: string;
  display_name: string;
  description: string;
  config: ICompilationTemplateConfig;
}

export interface ICompilationTemplateGroup {
  id: string;
  name: string;
  description?: string;
  scope?: string;
  avatar?: string;
  create_time?: number;
  update_time?: number;
  templates: ICompilationTemplate[];
}

export interface IWikiPreset {
  id: string;
  topic: string;
  instruction: string;
  page_example: string;
}

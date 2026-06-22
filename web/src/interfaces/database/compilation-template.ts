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

export interface ICompilationTemplateConfig {
  kind?: string;
  llm_id?: string;
  entity?: ICompilationTemplateSection;
  relation?: ICompilationTemplateSection;
  global_rules?: string;
  [section: string]: ICompilationTemplateSection | string | undefined;
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

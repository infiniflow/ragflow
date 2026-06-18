export interface ICompilationTemplate {
  id: string;
  name: string;
  description: string;
  kind: string;
  config: Record<string, any>;
  create_time?: number;
  update_time?: number;
}

export interface ICompilationTemplateListResult {
  templates: ICompilationTemplate[];
  total: number;
}

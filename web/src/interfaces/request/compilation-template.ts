import {
  CompilationTemplateConfig,
  CompilationTemplateKind,
} from '../database/compilation-template';
import { IPaginationRequestBody } from './base';

export interface IListCompilationTemplatesRequest extends IPaginationRequestBody {
  kind?: CompilationTemplateKind;
}

export interface ICreateCompilationTemplateRequest {
  name: string;
  description?: string;
  kind: CompilationTemplateKind;
  config: CompilationTemplateConfig;
}

export interface IUpdateCompilationTemplateRequest extends Partial<ICreateCompilationTemplateRequest> {
  id: string;
}

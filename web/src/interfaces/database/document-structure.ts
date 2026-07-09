import { CompilationTemplateKind } from '@/constants/compilation';

export type StructureTemplateKind = CompilationTemplateKind | 'raptor';

export interface IStructureGraphEntity {
  id?: string;
  name?: string;
  aliases?: string[];
  description?: string;
  discription?: string;
  mention_count?: number;
  source_chunk_ids?: string[];
  type?: string;
}

export interface IStructureGraphRelation {
  from: string;
  to: string;
  type?: string;
}

export interface IStructureGraphTemplate {
  kind: StructureTemplateKind;
  template_id: string;
  template_name: string;
  entities: IStructureGraphEntity[];
  relations: IStructureGraphRelation[];
}

export interface IStructureGraphResponse {
  templates: IStructureGraphTemplate[];
}

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
  /** Leaf clusters only: claims sourced to this cluster's chunks. */
  claim_count?: number;
  /** page_index fact/conclusion only: gate-verified verbatim quotes. */
  evidence?: IClaimEvidence[];
}

export interface IClaimEvidence {
  quote: string;
  chunk_id: string;
  start?: number;
  end?: number;
}

export interface IClaimItem {
  name: string;
  description?: string;
  source_chunk_ids?: string[];
  type?: string;
  evidence?: IClaimEvidence[];
}

export interface IClaimsResponse {
  claims: IClaimItem[];
  total: number;
  offset: number;
  limit: number;
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
  total_entities?: number;
  returned_entities?: number;
}

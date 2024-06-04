import { Edge, Node } from 'reactflow';

export interface DSLComponentList {
  id: string;
  name: string;
}

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
}

export interface IBeginForm {
  prologue?: string;
}

export interface IRetrievalForm {
  similarity_threshold?: number;
  keywords_similarity_weight?: number;
  top_n?: number;
  top_k?: number;
  rerank_id?: string;
  empty_response?: string;
  kb_ids: string[];
}

export interface IGenerateForm {
  max_tokens?: number;
  temperature?: number;
  top_p?: number;
  presence_penalty?: number;
  frequency_penalty?: number;
  cite?: boolean;
  prompt: number;
  llm_id: string;
  parameters: { key: string; component_id: string };
}

export type NodeData = {
  label: string;
  color: string;
  form: IBeginForm | IRetrievalForm | IGenerateForm;
};

export interface IFlow {
  avatar: null;
  canvas_type: null;
  create_date: string;
  create_time: number;
  description: null;
  dsl: {
    answer: any[];
    components: DSLComponentList;
    graph: { nodes: Node[]; edges: Edge[] };
    history: any[];
    path: string[];
  };
  id: string;
  title: string;
  update_date: string;
  update_time: number;
  user_id: string;
}

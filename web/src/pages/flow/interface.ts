import { FormInstance } from 'antd';
import { Node } from 'reactflow';

export interface DSLComponentList {
  id: string;
  name: string;
}

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
  form?: FormInstance;
  node?: Node<NodeData>;
  nodeId?: string;
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
export interface ICategorizeItem {
  name: string;
  description?: string;
  examples?: string;
  to?: string;
}

export interface IGenerateParameter {
  id?: string;
  key: string;
  component_id?: string;
}

export type ICategorizeItemResult = Record<
  string,
  Omit<ICategorizeItem, 'name'>
>;
export interface ICategorizeForm extends IGenerateForm {
  category_description: ICategorizeItemResult;
}

export interface IRelevantForm extends IGenerateForm {
  yes: string;
  no: string;
}

export type NodeData = {
  label: string; // operator type
  name: string; // operator name
  color: string;
  form: IBeginForm | IRetrievalForm | IGenerateForm | ICategorizeForm;
};

export type IPosition = { top: number; right: number; idx: number };

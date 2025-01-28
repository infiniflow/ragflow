import { Edge, Node } from '@xyflow/react';
import { IReference, Message } from './chat';

export type DSLComponents = Record<string, IOperator>;

export interface DSL {
  components: DSLComponents;
  history: any[];
  path?: string[][];
  answer?: any[];
  graph?: IGraph;
  messages: Message[];
  reference: IReference[];
}

export interface IOperator {
  obj: IOperatorNode;
  downstream: string[];
  upstream: string[];
  parent_id?: string;
}

export interface IOperatorNode {
  component_name: string;
  params: Record<string, unknown>;
}

export declare interface IFlow {
  avatar?: null | string;
  canvas_type: null;
  create_date: string;
  create_time: number;
  description: null;
  dsl: DSL;
  id: string;
  title: string;
  update_date: string;
  update_time: number;
  user_id: string;
}

export interface IFlowTemplate {
  avatar: string;
  canvas_type: string;
  create_date: string;
  create_time: number;
  description: string;
  dsl: DSL;
  id: string;
  title: string;
  update_date: string;
  update_time: number;
}

export type ICategorizeItemResult = Record<
  string,
  Omit<ICategorizeItem, 'name'>
>;

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
  index: number;
}

export interface ICategorizeForm extends IGenerateForm {
  category_description: ICategorizeItemResult;
}

export interface IRelevantForm extends IGenerateForm {
  yes: string;
  no: string;
}

export interface ISwitchCondition {
  items: ISwitchItem[];
  logical_operator: string;
  to: string;
}

export interface ISwitchItem {
  cpn_id: string;
  operator: string;
  value: string;
}

export interface ISwitchForm {
  conditions: ISwitchCondition[];
  end_cpn_id: string;
  no: string;
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

export type BaseNodeData<TForm extends any> = {
  label: string; // operator type
  name: string; // operator name
  color?: string;
  form?: TForm;
};

export type BaseNode<T = any> = Node<BaseNodeData<T>>;

export type IBeginNode = BaseNode<IBeginForm>;
export type IRetrievalNode = BaseNode<IRetrievalForm>;
export type IGenerateNode = BaseNode<IGenerateForm>;
export type ICategorizeNode = BaseNode<ICategorizeForm>;
export type ISwitchNode = BaseNode<ISwitchForm>;
export type IRagNode = BaseNode;
export type IRelevantNode = BaseNode;
export type ILogicNode = BaseNode;
export type INoteNode = BaseNode;
export type IMessageNode = BaseNode;
export type IRewriteNode = BaseNode;
export type IInvokeNode = BaseNode;
export type ITemplateNode = BaseNode;
export type IEmailNode = BaseNode;
export type IIterationNode = BaseNode;
export type IIterationStartNode = BaseNode;
export type IKeywordNode = BaseNode;

export type RAGFlowNodeType =
  | IBeginNode
  | IRetrievalNode
  | IGenerateNode
  | ICategorizeNode
  | ISwitchNode
  | IRagNode
  | IRelevantNode
  | ILogicNode
  | INoteNode
  | IMessageNode
  | IRewriteNode
  | IInvokeNode
  | ITemplateNode
  | IEmailNode
  | IIterationNode
  | IIterationStartNode
  | IKeywordNode;

export interface IGraph {
  nodes: RAGFlowNodeType[];
  edges: Edge[];
}

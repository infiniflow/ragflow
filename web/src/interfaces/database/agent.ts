export interface ICategorizeItem {
  name: string;
  description?: string;
  examples?: { value: string }[];
  index: number;
  to: string[];
  uuid: string;
}

export type ICategorizeItemResult = Record<
  string,
  Omit<ICategorizeItem, 'name' | 'examples' | 'uuid'> & { examples: string[] }
>;

export interface ISwitchCondition {
  items: ISwitchItem[];
  logical_operator: string;
  to: string[];
}

export interface ISwitchItem {
  cpn_id: string;
  operator: string;
  value: string;
}

export interface ISwitchForm {
  conditions: ISwitchCondition[];
  end_cpn_ids: string[];
  no: string;
}

import { AgentCategory } from '@/constants/agent';
import { Edge, Node } from '@xyflow/react';
import { IReference, Message } from './chat';

export type DSLComponents = Record<string, IOperator>;

export interface DSL {
  components: DSLComponents;
  history: any[];
  path?: string[];
  answer?: any[];
  graph?: IGraph;
  messages?: Message[];
  reference?: IReference[];
  globals: Record<string, any>;
  variables: Record<string, GlobalVariableType>;
  retrieval: IReference[];
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
  avatar?: string;
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
  permission: string;
  nickname: string;
  operator_permission: number;
  canvas_category: string;
}

export interface IFlowTemplate {
  avatar: string;
  canvas_type: string;
  create_date: string;
  create_time: number;
  canvas_category?: string;
  dsl: DSL;
  id: string;
  update_date: string;
  update_time: number;
  description: {
    en: string;
    zh: string;
  };
  title: {
    en: string;
    zh: string;
  };
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

export interface ICategorizeForm extends IGenerateForm {
  category_description: ICategorizeItemResult;
  items: ICategorizeItem[];
}

export interface IRelevantForm extends IGenerateForm {
  yes: string;
  no: string;
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

export interface ICodeForm {
  arguments: Record<string, string>;
  lang: string;
  script?: string;
  outputs: Record<string, { value: string; type: string }>;
}

export interface IAgentForm {
  sys_prompt: string;
  prompts: Array<{
    role: string;
    content: string;
  }>;
  max_retries: number;
  delay_after_error: number;
  visual_files_var: string;
  max_rounds: number;
  exception_method: Nullable<'comment' | 'go'>;
  exception_comment: any;
  exception_goto: any;
  tools: Array<{
    name: string;
    component_name: string;
    params: Record<string, any>;
  }>;
  mcp: Array<{
    mcp_id: string;
    tools: Record<string, Record<string, any>>;
  }>;
  outputs: {
    structured_output: Record<string, Record<string, any>>;
    content: Record<string, any>;
  };
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
export type ICodeNode = BaseNode<ICodeForm>;
export type IAgentNode = BaseNode;
export type IToolNode = BaseNode<IAgentForm>;

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

export interface ITraceData {
  component_id: string;
  trace: Array<Record<string, any>>;
}

export interface IAgentLogResponse {
  id: string;
  message: IAgentLogMessage[];
  update_date: string;
  create_date: string;
  update_time: number;
  create_time: number;
  round: number;
  thumb_up: number;
  errors: string;
  source: string;
  user_id: string;
  dsl: string;
  reference: IReference;
}
export interface IAgentLogsResponse {
  total: number;
  sessions: IAgentLogResponse[];
}
export interface IAgentLogsRequest {
  keywords?: string;
  to_date?: string | Date;
  from_date?: string | Date;
  orderby?: string;
  desc?: boolean;
  page?: number;
  page_size?: number;
}

export interface IAgentLogMessage {
  content: string;
  role: 'user' | 'assistant';
  id: string;
}

export interface IPipeLineListRequest {
  page?: number;
  page_size?: number;
  keywords?: string;
  orderby?: string;
  desc?: boolean;
  canvas_category?: AgentCategory;
}

export interface GlobalVariableType {
  name: string;
  value: any;
  description: string;
  type: string;
}

import { Edge, Node } from 'reactflow';
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
}

export interface IOperatorNode {
  component_name: string;
  params: Record<string, unknown>;
}

export interface IGraph {
  nodes: Node[];
  edges: Edge[];
}

export interface IFlow {
  avatar: null;
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

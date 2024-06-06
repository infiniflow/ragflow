import { Edge, Node } from 'reactflow';

export type DSLComponents = Record<string, IOperator>;

export interface DSL {
  components: DSLComponents;
  history?: any[];
  path?: string[];
  answer?: any[];
  graph?: IGraph;
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
  dsl: {
    answer: any[];
    components: DSLComponents;
    graph: IGraph;
    history: any[];
    path: string[];
  };
  id: string;
  title: string;
  update_date: string;
  update_time: number;
  user_id: string;
}

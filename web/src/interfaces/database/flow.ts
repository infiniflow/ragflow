export type DSLComponents = Record<string, Operator>;

export interface DSL {
  components: DSLComponents;
  history: any[];
  path: string[];
  answer: any[];
}

export interface Operator {
  obj: OperatorNode;
  downstream: string[];
  upstream: string[];
}

export interface OperatorNode {
  component_name: string;
  params: Record<string, unknown>;
}

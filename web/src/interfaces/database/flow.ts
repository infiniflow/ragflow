export type DSLComponents = Record<string, IOperator>;

export interface DSL {
  components: DSLComponents;
  history: any[];
  path: string[];
  answer: any[];
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

import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { FormInstance } from 'antd';

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
  form?: FormInstance;
  node?: RAGFlowNodeType;
  nodeId?: string;
}

export interface INextOperatorForm {
  node?: RAGFlowNodeType;
  nodeId?: string;
}

export interface IGenerateParameter {
  id?: string;
  key: string;
  component_id?: string;
}

export interface IInvokeVariable extends IGenerateParameter {
  value?: string;
}

export type IPosition = { top: number; right: number; idx: number };

export interface BeginQuery {
  key: string;
  type: string;
  value: string;
  optional: boolean;
  name: string;
  options: (number | string | boolean)[];
}

export type IInputs = {
  avatar: string;
  title: string;
  inputs: Record<string, BeginQuery>;
  prologue: string;
};

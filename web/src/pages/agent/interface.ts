import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { FormInstance } from 'antd';
import { UseFormReturn } from 'react-hook-form';

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
  form?: FormInstance;
  node?: RAGFlowNodeType;
  nodeId?: string;
}

export interface INextOperatorForm {
  form: UseFormReturn;
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

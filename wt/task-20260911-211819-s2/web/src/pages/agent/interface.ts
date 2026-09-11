import { FormInstance } from '@/interfaces/antd-compat';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { FieldErrors } from 'react-hook-form';

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
  form?: FormInstance;
  node?: RAGFlowNodeType;
  nodeId?: string;
}

export interface INextOperatorForm {
  node?: RAGFlowNodeType;
  nodeId?: string;
  onValuesChange?(values: any): void;
  hideOutputs?: boolean;
  // Validation errors produced for this operator by an outer form's schema
  // (parser_config.<operatorId>), mirrored onto the fields of this form.
  externalErrors?: FieldErrors;
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
  mode: string;
};

export type IOutputs = Record<
  string,
  {
    type?: string;
    value?: string;
  }
>;

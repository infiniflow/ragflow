import { IConversation, Message } from '@/interfaces/database/chat';
import { FormInstance } from 'antd';

export interface ISegmentedContentProps {
  show: boolean;
  form: FormInstance;
}

export interface IVariable {
  temperature: number;
  top_p: number;
  frequency_penalty: number;
  presence_penalty: number;
  max_tokens: number;
}

export interface VariableTableDataType {
  key: string;
  variable: string;
  optional: boolean;
}

export type IPromptConfigParameters = Omit<VariableTableDataType, 'variable'>;

export interface IMessage extends Message {
  id: string;
}

export interface IClientConversation extends IConversation {
  message: IMessage[];
}

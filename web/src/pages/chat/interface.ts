import { IConversation, IReference, Message } from '@/interfaces/database/chat';
import { FormInstance } from 'antd';

export interface ISegmentedContentProps {
  show: boolean;
  form: FormInstance;
  setHasError: (hasError: boolean) => void;
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
  reference?: IReference; // the latest news has reference
}

export interface IClientConversation extends IConversation {
  message: IMessage[];
}

export interface IDialog {
  avatar?: string;
  create_date?: string;
  create_time?: number;
  description?: string;
  id?: string;
  kb_ids?: string[];
  llm_id?: string;
  llm_setting?: ILlmSetting;
  name?: string;
  prompt_config?: IPromptConfigParameters;
  prompt_type?: string;
  similarity_threshold?: number;
  status?: string;
  top_k?: number;
  top_n?: number;
  update_date?: string;
  update_time?: number;
  vector_similarity_weight?: number;
  memory_config?: IMemoryConfig;
}

export interface IMemoryConfig {
  enabled: boolean;
  max_memories: number;
  threshold: number;
  store_interval: number;
  min_message_length?: number;
}

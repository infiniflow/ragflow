import { MessageType } from '@/constants/chat';
import { IChunk } from './knowledge';

export interface PromptConfig {
  empty_response: string;
  parameters: Parameter[];
  prologue: string;
  system: string;
}

export interface Parameter {
  key: string;
  optional: boolean;
}

export interface LlmSetting {
  Creative: Variable;
  Custom: Variable;
  Evenly: Variable;
  Precise: Variable;
}

export interface Variable {
  frequency_penalty: number;
  max_tokens: number;
  presence_penalty: number;
  temperature: number;
  top_p: number;
}

export interface IDialog {
  create_date: string;
  create_time: number;
  description: string;
  icon: string;
  id: string;
  kb_ids: string[];
  kb_names: string[];
  language: string;
  llm_id: string;
  llm_setting: LlmSetting;
  llm_setting_type: string;
  name: string;
  prompt_config: PromptConfig;
  prompt_type: string;
  status: string;
  tenant_id: string;
  update_date: string;
  update_time: number;
}

export interface IConversation {
  create_date: string;
  create_time: number;
  dialog_id: string;
  id: string;
  message: Message[];
  reference: IReference[];
  name: string;
  update_date: string;
  update_time: number;
}

export interface Message {
  content: string;
  role: MessageType;
}

export interface IReference {
  chunks: IChunk[];
  doc_aggs: Docagg[];
  total: number;
}

export interface Docagg {
  count: number;
  doc_id: string;
  doc_name: string;
}

// interface Chunk {
//   chunk_id: string;
//   content_ltks: string;
//   content_with_weight: string;
//   doc_id: string;
//   docnm_kwd: string;
//   img_id: string;
//   important_kwd: any[];
//   kb_id: string;
//   similarity: number;
//   term_similarity: number;
//   vector_similarity: number;
// }

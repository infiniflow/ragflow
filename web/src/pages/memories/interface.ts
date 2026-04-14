export interface CreateMemoryResponse {
  id: string;
  name: string;
  description: string;
}

export interface MemoryListParams {
  keywords?: string;
  parser_id?: string;
  page?: number;
  page_size?: number;
  orderby?: string;
  desc?: boolean;
  owner_ids?: string;
}
export type MemoryType = 'raw' | 'semantic' | 'episodic' | 'procedural';
export type StorageType = 'table' | 'graph';
export type Permissions = 'me' | 'team';
export type ForgettingPolicy = 'FIFO' | 'LRU';
export interface ICreateMemoryProps {
  name: string;
  memory_type: MemoryType[];
  embd_id: string;
  llm_id: string;
}
export interface IMemory extends ICreateMemoryProps {
  id: string;
  avatar: string;
  tenant_id: string;
  owner_name: string;
  storage_type: StorageType;
  permissions: Permissions;
  description: string;
  memory_size: number;
  forgetting_policy: ForgettingPolicy;
  temperature: string;
  system_prompt: string;
  user_prompt: string;
  create_date: string;
  create_time: number;
}
export interface MemoryListResponse {
  code: number;
  data: {
    memory_list: Array<IMemory>;
    total_count: number;
  };
  message: string;
}

export interface DeleteMemoryProps {
  memory_id: string;
}

export interface DeleteMemoryResponse {
  code: number;
  data: boolean;
  message: string;
}

export interface IllmSettingProps {
  llm_id: string;
  parameter: string;
  temperature?: number;
  top_p?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
}
interface IllmSettingEnableProps {
  temperatureEnabled?: boolean;
  topPEnabled?: boolean;
  presencePenaltyEnabled?: boolean;
  frequencyPenaltyEnabled?: boolean;
}
export interface IMemoryAppDetailProps {
  avatar: any;
  created_by: string;
  description: string;
  id: string;
  name: string;
  memory_config: {
    cross_languages: string[];
    doc_ids: string[];
    chat_id: string;
    highlight: boolean;
    kb_ids: string[];
    keyword: boolean;
    query_mindmap: boolean;
    related_memory: boolean;
    rerank_id: string;
    use_rerank?: boolean;
    similarity_threshold: number;
    summary: boolean;
    llm_setting: IllmSettingProps & IllmSettingEnableProps;
    top_k: number;
    use_kg: boolean;
    vector_similarity_weight: number;
    web_memory: boolean;
    chat_settingcross_languages: string[];
    meta_data_filter?: {
      method: string;
      manual: { key: string; op: string; value: string }[];
    };
  };
  tenant_id: string;
  update_time: number;
}

export interface MemoryDetailResponse {
  code: number;
  data: IMemoryAppDetailProps;
  message: string;
}

// export type IUpdateMemoryProps = Omit<IMemoryAppDetailProps, 'id'> & {
//   id: string;
// };

export interface IThirdOAIModel {
  available: boolean;
  create_date: string;
  create_time: number;
  fid: string;
  id: number;
  llm_name: string;
  max_tokens: number;
  model_type: string;
  status: string;
  tags: string;
  update_date: string;
  update_time: number;
  tenant_id?: string;
  tenant_name?: string;
  is_tools: boolean;
}

export type IThirdOAIModelCollection = Record<string, IThirdOAIModel[]>;

export interface IFactory {
  create_date: string;
  create_time: number;
  logo: string;
  name: string;
  status: string;
  tags: string;
  update_date: string;
  update_time: number;
}

export interface IMyLlmValue {
  llm: Llm[];
  tags: string;
}

export interface Llm {
  name: string;
  type: string;
  status: '0' | '1';
  used_token: number;
}

export interface IDynamicModel {
  id: string;
  llm_name: string;
  name: string;
  model_type: string;
  provider: string;
  max_tokens: number;
  is_tools: boolean;
  supports_vision?: boolean;
  pricing?: {
    prompt: number;
    completion: number;
  };
  tags: string;
  architecture?: Record<string, any>;
}

export interface IFactoryModelsResponse {
  factory: string;
  models: IDynamicModel[];
  models_by_category?: Record<string, IDynamicModel[]>;
  supported_categories?: string[];
  default_base_url?: string | null;
  is_dynamic: boolean;
  cached?: boolean;
}

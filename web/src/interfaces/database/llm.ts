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

export interface IAvailableProvider {
  name: string;
  model_types: string[];
  url: { default?: string; [key: string]: string | undefined };
}

export interface IProviderInstance {
  api_key: string;
  id: string;
  instance_name: string;
  provider_id: string;
  region: string;
  status: string;
  /**
   * Optional: only returned by the showProviderInstance endpoint. Used
   * to pre-fill the base_url/api_base form field in the ProviderModal
   * (e.g. when opening an existing instance in viewMode).
   */
  base_url?: string;
}
export interface IAddedModel {
  model_type: string[];
  name: string;
  provider_id: string;
  provider_name: string;
  instance_id: string;
  instance_name: string;
}

export interface IInstanceModel {
  max_tokens: number;
  model_type: string[];
  name: string;
  status: string;
}

export interface IDefaultModel {
  enable: boolean;
  model_instance: string;
  model_name: string;
  model_provider: string;
  model_type: string;
}

export interface IAddLlmRequestBody {
  llm_factory: string; // Ollama
  // model_name: string;
  // model_type: string | string[];
  base_url?: string; // chat|embedding|speech2text|image2text
  api_key?: string | Record<string, any>;
  max_tokens: number;
  is_tools?: boolean;
  region?: string;
  model_info: IModelInfo[];
}

export interface IModelInfo {
  model_name: string;
  model_type: string | string[];
  max_tokens: number;
  /**
   * Per-model extras (e.g. `is_tools` derived from the model descriptor's
   * `features`). Optional for backward compatibility with legacy
   * single-model payloads.
   */
  extra?: Record<string, any>;
}

export interface IDeleteLlmRequestBody {
  llm_factory: string; // Ollama
  llm_name?: string;
}

export interface IListProvidersRequestParams {
  available?: boolean;
}

export interface IAddProviderRequestBody {
  provider_name: string;
}

export type IAddProviderInstanceRequestBody = IAddLlmRequestBody & {
  instance_name: string;
  region?: string;
};

export interface IDeleteProviderInstanceRequestBody {
  provider_name: string;
  instances: string[];
}

export interface IShowProviderInstanceRequestParams {
  provider_name: string;
  instance_name: string;
}

export interface IAddInstanceModelRequestBody {
  model_name: string;
  model_type: string[];
  max_tokens: number;
  extra?: Record<string, any>;
}

export interface IListAllModelsRequestParams {
  type?: string;
}

export interface IUpdateModelStatusRequestBody {
  provider_name: string;
  instance_name: string;
  model_name: string;
  status: 'active' | 'inactive';
}

export interface ISetDefaultModelRequestBody {
  model_provider: string;
  model_instance: string;
  model_type: string;
  model_name: string;
}

/**
 * Item shape returned by the list-provider-models endpoint.
 * Fields match the backend's available-model descriptor.
 */
export interface IProviderModelItem {
  name: string;
  max_tokens: number;
  model_types: string[];
  features: string[] | null;
}

/**
 * Request payload for the list-provider-models endpoint.
 * Mirrors the verifyProviderConnection payload so the same form
 * fields (api_key, base_url, region, model_info) can be reused.
 */
export interface IListProviderModelsRequestBody {
  provider_name: string;
  api_key: string;
  base_url?: string;
  region?: string;
  model_info?: IModelInfo[];
}

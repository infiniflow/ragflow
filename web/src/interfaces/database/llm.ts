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
  has_instance: boolean;
}

export interface IProviderInstance {
  /**
   * Usually a plain key string, but the showProviderInstance endpoint
   * may return an object `{ api_key, ...nested }` for providers that
   * bundle extra credentials (see the nested fields below).
   */
  api_key: string | Record<string, any>;
  id: string;
  instance_name: string;
  provider_id: string;
  region: string;
  status: string;
  /**
   * Optional: only returned by the showProviderInstance endpoint. Used
   * to pre-fill the base_url/api_base form field when opening a saved
   * instance.
   */
  base_url?: string;
  /**
   * Provider-specific credentials that may be returned either at the top
   * level or nested inside `api_key`:
   *   - group_id       → MiniMax
   *   - api_version    → Azure OpenAI
   *   - provider_order → OpenRouter
   */
  group_id?: string;
  api_version?: string;
  provider_order?: string;
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
  /**
   * Persisted verification result from the backend:
   *   - `true`  → verified successfully
   *   - `false` → verified but failed
   *   - `undefined` → never verified yet
   */
  verify?: 'unknown' | 'success' | 'fail';
  /**
   * Persisted Tool-call flag from `tenant_model.extra.is_tools`.
   * The backend's `_hybrid_get_instance_models` includes this so the
   * frontend can forward the correct value in auto-save payloads
   * without relying solely on the (possibly unfetched) catalog.
   */
  is_tools?: boolean;
}

export interface IDefaultModel {
  enable: boolean;
  model_instance: string;
  model_name: string;
  model_provider: string;
  model_type: string;
}

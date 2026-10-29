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
  /**
   * Display-only example endpoint from the provider catalog
   * (`conf/models/*.json`). Rendered as the endpoint input's placeholder;
   * never used to build requests nor to pre-fill the form.
   */
  url_hint?: string;
}

export interface IProviderInstance {
  id: string;
  instance_name: string;
  provider_id: string;
  region: string;
  status: string;
  /** Whether the current user owns this model instance. */
  is_owner?: boolean;
  /** Whether the server has a credential configured for this instance. */
  api_key_configured?: boolean;
  /** Safe provider configuration values; secret values are never returned. */
  api_key_config?: Record<string, any>;
  /** Names of configured credential fields, without their values. */
  credential_status?: Record<string, boolean>;
  /** Masked credential previews; actual credential values are never returned. */
  credential_masks?: Record<string, string>;
  /**
   * Optional: only returned by the showProviderInstance endpoint. Used
   * to pre-fill the base_url form field when opening a saved
   * instance.
   */
  base_url?: string;
  /** Safe provider-specific settings returned separately from secrets. */
  group_id?: string;
  api_version?: string;
  provider_order?: string;
}
export interface IAddedModel {
  model_id: string;
  status?: string;
  instance_status?: string;
  model_type: string[];
  name: string;
  provider_id: string;
  provider_name: string;
  instance_id: string;
  instance_name: string;
  tenant_id?: string;
  tenant_name?: string;
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
  /**
   * Per-model extra config persisted in `tenant_model.extra`.
   * Carries provider-specific fields such as SoMark's element-format
   * selects and feature-config toggles. Echoed back by the backend's
   * `_hybrid_get_instance_models` so the frontend can pre-fill the
   * edit dialog.
   */
  extra?: Record<string, any>;
}

export interface IDefaultModel {
  model_id: string;
  enable: boolean;
  model_instance: string;
  model_name: string;
  model_provider: string;
  model_type: string;
}

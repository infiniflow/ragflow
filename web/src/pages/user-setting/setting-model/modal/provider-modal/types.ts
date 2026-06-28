import { FormFieldType } from '@/components/dynamic-form';
import type { IModelInfo } from '@/interfaces/request/llm';
import type { ReactNode } from 'react';

/**
 * Form field types.
 * - `FormFieldType.*` values map 1:1 to DynamicForm's field types.
 * - `'inputSelect'` is a project-specific token: a text input combined with a
 *   dropdown of suggested values. The ProviderModal resolves it into
 *   `FormFieldType.Custom` with a custom `render` function.
 */
export type FieldType = FormFieldType | 'inputSelect';

/**
 * String tokens for shouldRender
 * The component resolves these into actual functions based on runtime context (instanceNameSet, etc.)
 */
export type ShouldRenderToken =
  | 'hideWhenInstanceExists'
  | 'modelTypeIncludesChat'
  | 'modelTypeSupportsToolCall'
  | 'modelTypeIncludesTtsAndNotExists'
  | 'showBaseUrl'
  | 'showGroupId';

/**
 * Option label can be a string or ReactNode (used for rich-text labels in InputSelect).
 * `regionKey` is the original key from the provider's `url` object (e.g. 'default',
 * 'intl', 'cn'). It is preserved on the option so that the modal can map the
 * currently selected URL back to its key for the `region` submit field.
 */
export type SelectOption = {
  label: string | ReactNode;
  value: string;
  regionKey?: string;
};

/**
 * Resolver for a text value that may differ by factory (provider).
 * Use when a shared FieldConfig entry needs different i18n keys per provider
 * (e.g. the generic `base_url` field renders different tooltip / placeholder
 * for Minimax, TongYiQianWen, SILICONFLOW, etc.).
 */
export type FactoryTextResolver = (llmFactory: string) => string;

/**
 * Field config: defines the presentation and behavior of a form field
 */
export interface FieldConfig {
  /** Field name (supports nested paths, e.g. 'model_info.model_type') */
  name: string;
  /** Label i18n key */
  label: string;
  /** Field type */
  type: FieldType;
  /** Whether the field is required */
  required?: boolean;
  /**
   * Placeholder i18n key. May be a static key, or a function that takes the
   * current `llmFactory` and returns the key (for per-provider placeholders).
   */
  placeholder?: string | FactoryTextResolver;
  /**
   * Tooltip i18n key. May be a static key, or a function that takes the
   * current `llmFactory` and returns the key (for per-provider tooltips).
   */
  tooltip?: string | FactoryTextResolver;
  /** Options (used for select/multiSelect/inputSelect) */
  options?: SelectOption[];
  /** Default value */
  defaultValue?: any;
  /**
   * Validation rules.
   * `message` is treated as an i18n key by the ProviderModal and translated
   * via `t()` at field-build time. In `Number` fields, `min` / `max` bound
   * the value; the message is shown when the bound is violated.
   */
  validation?: {
    min?: number;
    max?: number;
    message?: string;
  };
  /**
   * Conditional rendering: returns true to show the field
   * @param values current form values
   */
  shouldRender?: ((values: Record<string, any>) => boolean) | ShouldRenderToken;
}

/**
 * Provider config: defines the full behavior of a LLM provider modal
 */
export interface ProviderConfig {
  /** Corresponding LLMFactory value (also used as the field-config key) */
  llmFactory: string;
  /** Modal title */
  title: string;
  /** Field list (in render order) */
  fields: FieldConfig[];
  /**
   * Transform form values into verify API parameters
   * Used to construct api_key / base_url / region / model_info when the Verify button is clicked.
   * `modelInfo` is assembled from `values` by the transform itself: if `values.model_info`
   * is already an array (the picker-merged case), it is used as-is; otherwise the transform
   * falls back to assembling from individual form fields (model_name / model_type / max_tokens / is_tools).
   */
  verifyTransform?: (values: Record<string, any>) => {
    apiKey: string | object | Record<string, any>;
    baseUrl?: string;
    region?: string;
    modelInfo?: IModelInfo[];
  };
  /**
   * Transform form values into submit API parameters.
   * Used to handle special field name mapping (e.g. volcengine's endpoint_id -> ark_api_key).
   * `modelInfo` is assembled from `values` by the transform itself (same rules as verifyTransform).
   */
  submitTransform?: (values: Record<string, any>) => Record<string, any>;
  /**
   * Optional link at the bottom of the modal
   * e.g. the official documentation link for Ollama-family providers
   */
  docLink?: string;
  /**
   * i18n key for the docLink text (optional)
   * e.g. 'ollamaLink'; the { name: llmFactory } variable is passed in
   */
  docLinkI18nKey?: string;
  /**
   * Custom docLink text (optional, takes precedence over docLinkI18nKey)
   */
  docLinkText?: string;
}

/**
 * Payload for the viewMode save callback. The modal calls `onViewModeOk`
 * (when provided) instead of `onOk` whenever `viewMode` is true.
 *
 * - `instanceName` is the pre-existing instance's name (taken from
 *   `initialValues.instance_name`).
 * - `llmFactory` is the current provider/factory.
 * - For LIST_MODEL_PROVIDERS, `modelInfos` carries the full list of
 *   currently checked models in the picker (one IModelInfo per checked
 *   item) and `formValues` is undefined.
 * - For non-LIST_MODEL_PROVIDERS, the picker is hidden so `modelInfos`
 *   is empty and `formValues` carries the editable model-related form
 *   values (model_name, model_type, max_tokens, is_tools).
 */
export interface IViewModeOkPayload {
  instanceName: string;
  llmFactory: string;
  modelInfos: IModelInfo[];
  formValues?: Record<string, any>;
}

/**
 * ProviderModal component props
 */
export interface ProviderModalProps {
  visible: boolean;
  hideModal: () => void;
  llmFactory: string;
  loading: boolean;
  editMode?: boolean;
  /**
   * Read-only "edit models" mode: opens the modal pre-filled with an
   * existing instance's data and only allows editing the model-related
   * fields (model_name, model_type, max_tokens, is_tools) plus the
   * list-models picker (when applicable). All other fields are disabled.
   * On save, only `addInstanceModel` is invoked (not `addProviderInstance`).
   */
  viewMode?: boolean;
  initialValues?: Record<string, any>;
  /**
   * Base URL options for the input+select combo (from IAvailableProvider.url)
   * Used by base_url/api_base fields of type inputSelect
   */
  baseUrlOptions?: SelectOption[];
  onOk: (payload: any, isVerify?: boolean) => Promise<any>;
  onVerify: (payload: any) => Promise<any>;
  /**
   * Save handler used when `viewMode` is true. The modal calls this with
   * the list of selected models (LIST_MODEL_PROVIDERS) or the editable
   * model-related form values (non-LIST_MODEL_PROVIDERS). If omitted,
   * the modal falls back to `onOk` and submits the standard payload.
   */
  onViewModeOk?: (payload: IViewModeOkPayload) => Promise<any>;
}

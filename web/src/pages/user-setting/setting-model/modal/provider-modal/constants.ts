import { LLMFactory } from '@/constants/llm';

/**
 * Provider factories that opt into the "List Models" picker UI.
 *
 * For these factories, the modal hides the traditional model_name,
 * model_type, max_tokens, and is_tools form fields and instead shows a
 * "List Models" button that fetches available models from the provider's
 * `/providers/<factory>/models` endpoint. The user can multi-select models
 * from the response; each selected model is converted to an `IModelInfo`
 * entry and submitted as `model_info`.
 *
 * For all other factories the picker is hidden and the form renders the
 * 4 model_* fields directly.
 */
export const LIST_MODEL_PROVIDERS = new Set<string>([
  LLMFactory.Ollama,
  LLMFactory.OpenRouter,
  LLMFactory.VLLM,
  LLMFactory.OpenAiAPICompatible,
  LLMFactory.LMStudio,
  LLMFactory.VolcEngine,
  LLMFactory.Xinference,
  LLMFactory.LocalAI,
  LLMFactory.BaiduYiYan,

  // LLMFactory.HuggingFace,
  // LLMFactory.GoogleCloud,
  // LLMFactory.TencentCloud,
  // LLMFactory.XunFeiSpark,
  // LLMFactory.GPUStack,
  // LLMFactory.FishAudio,
  // LLMFactory.MinerU,
  // LLMFactory.PaddleOCR,
]);

/**
 * The set of form-field names that are owned by the list-models picker
 * (not registered in the dynamic form when the picker is active).
 *
 * Doubles as the whitelist of fields that remain editable in viewMode —
 * in viewMode every other field is disabled so only model-related edits
 * are possible.
 */
export const LIST_MODEL_FIELD_NAMES = new Set<string>([
  'model_name',
  'model_type',
  'max_tokens',
  'is_tools',
]);

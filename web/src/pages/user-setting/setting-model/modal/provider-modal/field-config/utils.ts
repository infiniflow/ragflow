import { IModelInfo } from '@/interfaces/request/llm';

/**
 * Capitalize the first letter of a string
 */
export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * When model_type contains chat and vision=true, automatically add image2text
 */
export function applyChatToImage2Text(
  modelType: string[] | string | undefined,
  vision?: boolean,
): string[] {
  const arr = Array.isArray(modelType)
    ? modelType
    : modelType
      ? [modelType]
      : [];
  if (arr.includes('chat') && vision) {
    return [...arr, 'image2text'];
  }
  return arr;
}

/**
 * Build the IModelInfo[] payload for verify/submit from the form values.
 *
 * Resolution order:
 * 1. If `values.model_info` is a non-empty array (the picker-merged case,
 *    populated by the call site before invoking the transform), use it as-is.
 * 2. Otherwise, assemble a single-entry array from the individual form
 *    fields (`model_name`, `model_type`, `max_tokens`, plus `is_tools` /
 *    `vision` placed under `extra.is_tools` when present).
 * 3. If `model_name` is missing, return an empty array — the caller can
 *    decide whether to short-circuit (most providers require a model name).
 */
export const buildModelInfoFromValues = (
  values: Record<string, any>,
): IModelInfo[] => {
  if (Array.isArray(values.model_info) && values.model_info.length > 0) {
    return values.model_info;
  }
  if (!values.model_name) return [];
  const is_tools = values.is_tools ?? values.vision;
  const entry: IModelInfo = {
    model_name: values.model_name,
    model_type: values.model_type ?? [],
    max_tokens: values.max_tokens ?? 0,
  };
  if (is_tools !== undefined) {
    entry.extra = { is_tools };
  }
  return [entry];
};

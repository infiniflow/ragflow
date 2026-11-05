/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IModelInfo } from '@/interfaces/request/llm';

/**
 * Capitalize the first letter of a string
 */
export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * When model_type contains chat and vision=true, automatically add vision.
 */
export function applyChatToVision(
  modelType: string[] | string | undefined,
  vision?: boolean,
): string[] {
  const arr = Array.isArray(modelType)
    ? modelType
    : modelType
      ? [modelType]
      : [];
  if (arr.includes('chat') && vision) {
    return [...arr, 'vision'];
  }
  return arr;
}

/**
 * Parse the backend's safe provider config object. `echoTransform`
 * implementations use it to restore non-secret provider settings; secret
 * fields are filtered by the backend before this data reaches the browser.
 *
 * Returns `undefined` for bare strings or unparseable input so callers
 * can fall back to empty values via `??`.
 */
export function parseApiKeyAsObject(
  raw: unknown,
): Record<string, any> | undefined {
  let obj: any = raw;
  if (typeof raw === 'string') {
    const trimmed = raw.trim();
    if (!trimmed.startsWith('{')) return undefined;
    try {
      obj = JSON.parse(trimmed);
    } catch {
      return undefined;
    }
  }
  return obj && typeof obj === 'object' ? obj : undefined;
}

const credentialFieldPattern =
  /(api[_-]?key|secret|token|password|credential|private|access[_-]?key|role[_-]?arn|_(ak|sk|sid|key)$)/i;

function hasCredentialValue(value: unknown): boolean {
  if (typeof value === 'string') return value.trim() !== '';
  if (Array.isArray(value)) return value.some(hasCredentialValue);
  if (value && typeof value === 'object') {
    return Object.values(value as Record<string, unknown>).some(
      hasCredentialValue,
    );
  }
  return Boolean(value);
}

/** Whether a provider credential payload contains a non-empty credential. */
export function hasProviderCredentialInput(value: unknown): boolean {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return false;
    try {
      const parsed = JSON.parse(trimmed);
      if (parsed && typeof parsed === 'object') {
        return hasProviderCredentialInput(parsed);
      }
    } catch {
      // A non-JSON string is a plain API key.
    }
    return true;
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return false;
  }
  return Object.entries(value as Record<string, unknown>).some(
    ([key, fieldValue]) =>
      credentialFieldPattern.test(key) && hasCredentialValue(fieldValue),
  );
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

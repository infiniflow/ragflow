/*
 * Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { hasProviderCredentialInput } from '../provider-schema/field-config/utils';
import { SAVED_CREDENTIAL_MASK } from './saved-credential-input';

export const SavedProviderConfigFields = [
  'api_version',
  'auth_mode',
  'aws_role_arn',
  'bedrock_region',
  'fish_audio_refid',
  'google_project_id',
  'google_region',
  'group_id',
  'mineru_apiserver',
  'mineru_backend',
  'mineru_delete_output',
  'mineru_output_dir',
  'mineru_server_url',
  'opendataloader_apiserver',
  'paddleocr_algorithm',
  'paddleocr_api_url',
  'provider_order',
] as const;

export const SavedProviderConfigFieldSet = new Set<string>(
  SavedProviderConfigFields,
);

export function pickSavedProviderConfigOverrides(
  value: unknown,
): Record<string, unknown> {
  let config = value;
  if (typeof config === 'string') {
    try {
      config = JSON.parse(config);
    } catch {
      return {};
    }
  }
  if (!config || typeof config !== 'object' || Array.isArray(config)) return {};

  return Object.fromEntries(
    Object.entries(config as Record<string, unknown>).filter(
      ([key, fieldValue]) =>
        SavedProviderConfigFieldSet.has(key) &&
        fieldValue !== undefined &&
        fieldValue !== SAVED_CREDENTIAL_MASK,
    ),
  );
}

export function hasUnsavedProviderCredential(value: unknown): boolean {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return false;
    try {
      const parsed = JSON.parse(trimmed);
      if (parsed && typeof parsed === 'object') {
        return hasUnsavedProviderCredential(parsed);
      }
    } catch {
      return true;
    }
    return true;
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;

  return Object.entries(value as Record<string, unknown>).some(
    ([field, fieldValue]) =>
      !SavedProviderConfigFieldSet.has(field) &&
      hasProviderCredentialInput({ [field]: fieldValue }),
  );
}

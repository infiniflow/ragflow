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

import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import type { AddCustomModelDialogFields } from './add-custom-model-dialog';

/**
 * Single source of truth for the custom-model dialog schema. Mirrors
 * the shape of `IProviderModelItem` 1:1 — adding a new property to the
 * interface means adding an entry here, and the dialog auto-adapts.
 *
 * `label` and each option's `label` are i18n keys (under the `setting`
 * namespace). `useCustomModelFields` resolves them via `t()`.
 */
export const MODEL_FIELD_SCHEMA: AddCustomModelDialogFields[] = [
  {
    name: 'name',
    label: 'modelName',
    type: 'text',
    required: true,
    defaultValue: '',
  },
  {
    name: 'model_types',
    label: 'modelType',
    type: 'multi-select',
    required: false,
    defaultValue: [],
    options: [
      { value: 'chat', label: 'modelTypes.chat' },
      { value: 'embedding', label: 'modelTypes.embedding' },
      { value: 'rerank', label: 'modelTypes.rerank' },
      { value: 'tts', label: 'modelTypes.tts' },
      { value: 'image2text', label: 'modelTypes.image2text' },
      { value: 'speech2text', label: 'modelTypes.speech2text' },
    ],
  },
  {
    name: 'max_tokens',
    label: 'modelMaxTokens',
    type: 'number',
    required: false,
    min: 0,
    defaultValue: 0,
  },
  {
    name: 'features',
    label: 'modelFeatures',
    type: 'switch-group',
    required: false,
    defaultValue: [],
    options: [{ value: 'is_tools', label: 'modelFeatureToolCall' }],
  },
];

/**
 * Dialog field schema for adding a custom model. Returns
 * `MODEL_FIELD_SCHEMA` with i18n keys resolved.
 */
export const useCustomModelFields = (): AddCustomModelDialogFields[] => {
  const { t } = useTranslate('setting');

  return useMemo<AddCustomModelDialogFields[]>(
    () =>
      MODEL_FIELD_SCHEMA.map((field) => ({
        ...field,
        label: t(field.label),
        options: field.options?.map((opt) => ({
          value: opt.value,
          label: t(opt.label),
        })),
      })),
    [t],
  );
};

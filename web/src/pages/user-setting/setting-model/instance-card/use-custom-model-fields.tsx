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

import { LLMFactory } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import type { AddCustomModelDialogFields } from './add-custom-model-dialog';

/**
 * Single source of truth for the custom-model dialog schema. Mirrors
 * the shape of `IProviderModelItem` 1:1 - adding a new property to the
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
      { value: 'vision', label: 'modelTypes.image2text' },
      { value: 'ocr', label: 'modelTypes.ocr' },
      { value: 'asr', label: 'modelTypes.speech2text' },
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
 * SoMark-specific element-format selects appended to the model dialog
 * schema when the provider is SoMark. These fields are stored per-model
 * in `model_info[].extra`.
 *
 * `label` values are i18n keys under `setting.somark.*`. Option labels
 * use `somark.formatOptions.*` i18n keys.
 */
export const SOMARK_EXTRA_FIELDS: AddCustomModelDialogFields[] = [
  {
    name: 'elementFormats',
    label: 'somark.sectionElementFormats',
    type: 'section',
  },
  {
    name: 'somark_image_format',
    label: 'somark.imageFormat',
    type: 'select',
    defaultValue: 'url',
    options: [
      { value: 'url', label: 'somark.formatOptions.url' },
      { value: 'base64', label: 'somark.formatOptions.base64' },
      { value: 'none', label: 'somark.formatOptions.none' },
    ],
  },
  {
    name: 'somark_formula_format',
    label: 'somark.formulaFormat',
    type: 'select',
    defaultValue: 'latex',
    options: [
      { value: 'latex', label: 'somark.formatOptions.latex' },
      { value: 'mathml', label: 'somark.formatOptions.mathml' },
      { value: 'ascii', label: 'somark.formatOptions.ascii' },
    ],
  },
  {
    name: 'somark_table_format',
    label: 'somark.tableFormat',
    type: 'select',
    defaultValue: 'html',
    options: [
      { value: 'html', label: 'somark.formatOptions.html' },
      { value: 'markdown', label: 'somark.formatOptions.markdown' },
      { value: 'image', label: 'somark.formatOptions.image' },
    ],
  },
  {
    name: 'somark_cs_format',
    label: 'somark.csFormat',
    type: 'select',
    defaultValue: 'image',
    options: [{ value: 'image', label: 'somark.formatOptions.image' }],
  },
];

/**
 * SoMark feature-config options appended to the `features` switch-group
 * when the provider is SoMark. Selected entries (prefixed `somark_`) are
 * converted to boolean values in `model_info[].extra` by the dialog's
 * `handleSubmit`, so the data shape stays compatible with the legacy
 * `SoMarkInstanceCard` payload.
 */
export const SOMARK_FEATURE_OPTIONS: { value: string; label: string }[] = [
  {
    value: 'somark_enable_text_cross_page',
    label: 'somark.enableTextCrossPage',
  },
  {
    value: 'somark_enable_table_cross_page',
    label: 'somark.enableTableCrossPage',
  },
  {
    value: 'somark_enable_title_level_recognition',
    label: 'somark.enableTitleLevelRecognition',
  },
  { value: 'somark_enable_inline_image', label: 'somark.enableInlineImage' },
  { value: 'somark_enable_table_image', label: 'somark.enableTableImage' },
  {
    value: 'somark_enable_image_understanding',
    label: 'somark.enableImageUnderstanding',
  },
  { value: 'somark_keep_header_footer', label: 'somark.keepHeaderFooter' },
];

/**
 * Default features pre-selected for SoMark models. Mirrors the defaults
 * from the legacy `SoMarkInstanceCard` (table_image and image_understanding
 * default to `true`).
 */
export const SOMARK_DEFAULT_FEATURES = [
  'somark_enable_table_image',
  'somark_enable_image_understanding',
];

/**
 * Dialog field schema for adding a custom model. Returns
 * `MODEL_FIELD_SCHEMA` with i18n keys resolved. When `providerName`
 * is `SoMark`:
 *  - Appends `SOMARK_EXTRA_FIELDS` (element-format selects) after the
 *    base schema.
 *  - Replaces the `features` switch-group options with SoMark-only
 *    feature toggles (no "Tool Call") and pre-selects the defaults.
 */
export const useCustomModelFields = (
  providerName?: string,
): AddCustomModelDialogFields[] => {
  const { t } = useTranslate('setting');

  return useMemo<AddCustomModelDialogFields[]>(() => {
    const isSoMark = providerName === LLMFactory.SoMark;
    const schema = isSoMark
      ? [...MODEL_FIELD_SCHEMA, ...SOMARK_EXTRA_FIELDS]
      : MODEL_FIELD_SCHEMA;
    return schema.map((field) => {
      const mapped = {
        ...field,
        label: t(field.label),
        options: field.options?.map((opt) => ({
          value: opt.value,
          label: t(opt.label),
        })),
      };
      // For SoMark, replace the features switch-group options with
      // SoMark-only feature toggles (no "Tool Call") and pre-select
      // the default features.
      if (isSoMark && field.name === 'features') {
        mapped.options = SOMARK_FEATURE_OPTIONS.map((opt) => ({
          value: opt.value,
          label: t(opt.label),
        }));
        mapped.defaultValue = [...SOMARK_DEFAULT_FEATURES];
      }
      return mapped;
    });
  }, [t, providerName]);
};

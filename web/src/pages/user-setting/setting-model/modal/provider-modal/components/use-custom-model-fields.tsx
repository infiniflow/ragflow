import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { useMemo } from 'react';
import type { AddCustomModelDialogFields } from './add-custom-model-dialog';

/**
 * Allowed values for `IProviderModelItem['model_types']`. Kept in sync
 * with the backend model registry; `LabelKey` matches the i18n namespace
 * the descriptor below uses for each option.
 */
const MODEL_TYPE_VALUES = [
  'chat',
  'embedding',
  'rerank',
  'tts',
  'image2text',
  'speech2text',
] as const;

/**
 * Allowed values for `IProviderModelItem['features']`.
 */
const FEATURE_VALUES = ['tool_call'] as const;

/**
 * Descriptor for a single IProviderModelItem property. Each descriptor
 * tells the dialog how to render the form input for that property.
 *
 * - `type`     : the form input type
 * - `required` : whether the field must be non-empty
 * - `min`      : for `number` types, the minimum allowed value
 * - `options`  : for select-style types, the allowed value set
 * - `labelKey` : i18n key for the field label
 * - `optionLabelKey` : i18n key prefix for each option's label
 */
type ModelFieldDescriptor = {
  type: AddCustomModelDialogFields['type'];
  required: boolean;
  min?: number;
  options?: readonly string[];
  labelKey: string;
  optionLabelKey?: (value: string) => string;
};

/**
 * Single source of truth for the custom-model dialog schema. Mirrors
 * the shape of `IProviderModelItem` 1:1 — adding a new property to the
 * interface means adding an entry here, and the dialog auto-adapts.
 */
const MODEL_FIELD_SCHEMA: Record<
  keyof IProviderModelItem,
  ModelFieldDescriptor
> = {
  name: {
    type: 'text',
    required: true,
    labelKey: 'modelName',
  },
  model_types: {
    type: 'multi-select',
    required: false,
    options: MODEL_TYPE_VALUES,
    labelKey: 'modelType',
    optionLabelKey: (v) => `setting.modelTypes.${v}`,
  },
  max_tokens: {
    type: 'number',
    required: false,
    min: 0,
    labelKey: 'modelMaxTokens',
  },
  features: {
    type: 'switch-group',
    required: false,
    options: FEATURE_VALUES,
    labelKey: 'modelFeatures',
    optionLabelKey: (v) =>
      v === 'tool_call'
        ? 'modelFeatureToolCall'
        : v === 'function_call'
          ? 'modelFeatureFunctionCall'
          : v,
  },
};

/**
 * Dialog field schema for adding a custom model. Derived from
 * `IProviderModelItem` via `MODEL_FIELD_SCHEMA`, so the form is in
 * lockstep with the model interface.
 */
export const useCustomModelFields = (): AddCustomModelDialogFields[] => {
  const { t } = useTranslate('setting');

  return useMemo<AddCustomModelDialogFields[]>(() => {
    return (
      Object.entries(MODEL_FIELD_SCHEMA) as Array<
        [keyof IProviderModelItem, ModelFieldDescriptor]
      >
    ).map(([prop, desc]) => {
      const defaultValue =
        desc.type === 'number'
          ? 0
          : desc.type === 'multi-select' || desc.type === 'switch-group'
            ? []
            : '';

      return {
        name: String(prop),
        label: t(desc.labelKey),
        type: desc.type,
        required: desc.required,
        defaultValue,
        ...(desc.min !== undefined ? { min: desc.min } : {}),
        ...(desc.options
          ? {
              options: desc.options.map((value) => ({
                value,
                label: t(
                  desc.optionLabelKey ? desc.optionLabelKey(value) : value,
                ),
              })),
            }
          : {}),
      };
    });
  }, [t]);
};

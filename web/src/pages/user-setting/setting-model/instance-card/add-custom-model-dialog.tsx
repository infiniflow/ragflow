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

'use client';

import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { Loader2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef } from 'react';
import { z } from 'zod';

export interface AddCustomModelDialogFields {
  /** Field name (maps to IProviderModelItem key) */
  name: string;
  /** Display label */
  label: string;
  /** Form field type */
  type:
    | 'text'
    | 'number'
    | 'multi-select'
    | 'switch-group'
    | 'select'
    | 'switch'
    | 'section';
  /** Options for multi-select / switch-group / select types */
  options?: { label: string; value: string }[];
  /** Whether the field is required */
  required?: boolean;
  /** Default value */
  defaultValue?: unknown;
  /** Minimum value for number type */
  min?: number;
  /** Whether this field is disabled (non-editable) */
  disabled?: boolean;
}

interface AddCustomModelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Dialog title */
  title: React.ReactNode;
  /** Form field definitions */
  fields: AddCustomModelDialogFields[];
  /** Called when form is submitted with valid data */
  onSubmit: (item: IProviderModelItem) => void | Promise<void>;
  /** Submit button text */
  submitText?: React.ReactNode;
  /** Cancel button text */
  cancelText?: React.ReactNode;
  /** Loading state (submit in progress) */
  loading?: boolean;
  /** Existing model names for uniqueness validation */
  existingNames: string[];
  /**
   * Whitelist of feature keys that are provider-specific (e.g.
   * SoMark's `somark_enable_*`). On submit, these are moved from the
   * `features` array into `extra` as boolean `true` values so each
   * provider's payload stays self-describing. Standard features like
   * `is_tools` stay in `features`. When omitted, no features are
   * moved to `extra`.
   */
  providerFeatureKeys?: string[];
  /** Initial form values (overrides field-level defaults). Useful for edit mode. */
  defaultValues?: Record<string, unknown>;
}

type FormValues = Record<string, unknown>;

/**
 * Dynamic form dialog for adding a custom model.
 * Fields are driven by the `fields` prop and validated via Zod.
 */
export const AddCustomModelDialog = ({
  open,
  onOpenChange,
  title,
  fields,
  onSubmit,
  submitText,
  cancelText,
  loading = false,
  existingNames,
  defaultValues,
  providerFeatureKeys,
}: AddCustomModelDialogProps) => {
  const { t } = useTranslate('setting');
  const { t: commonT } = useTranslate('common');
  const formRef = useRef<DynamicFormRef>(null);

  // Translate AddCustomModelDialogFields -> FormFieldConfig for DynamicForm.
  // The custom `switch-group` type falls back to FormFieldType.Custom with
  // a render prop that re-implements the bordered switch list.
  const dynamicFields = useMemo<FormFieldConfig[]>(() => {
    return fields.map((field) => {
      const isArrayType =
        field.type === 'multi-select' || field.type === 'switch-group';
      const defaultValue =
        field.defaultValue ??
        (field.type === 'number'
          ? 0
          : field.type === 'switch'
            ? false
            : isArrayType
              ? []
              : '');

      if (field.type === 'section') {
        return {
          name: `__section_${field.name}`,
          label: field.label,
          type: FormFieldType.Custom,
          hideLabel: true,
          schema: z.any().optional(),
          render: () => (
            <div className="text-sm font-semibold text-muted-foreground border-b pb-1 mt-4">
              {field.label}
            </div>
          ),
        };
      }

      if (field.type === 'switch') {
        return {
          name: field.name,
          label: field.label,
          type: FormFieldType.Switch,
          required: field.required,
          defaultValue: defaultValue ?? false,
          disabled: field.disabled,
          labelClassName: '!mb-0',
        };
      }

      if (field.type === 'select') {
        return {
          name: field.name,
          label: field.label,
          type: FormFieldType.Select,
          required: field.required,
          defaultValue,
          disabled: field.disabled,
          options: field.options,
          placeholder: field.label,
        };
      }

      if (field.type === 'switch-group') {
        return {
          name: field.name,
          label: field.label,
          type: FormFieldType.Custom,
          required: field.required,
          defaultValue,
          disabled: field.disabled,
          schema: field.required
            ? z.array(z.string()).min(1, t('modelTypeRequired'))
            : z.array(z.string()).optional(),
          render: (fieldProps) => {
            const currentValues = (fieldProps.value as string[]) ?? [];
            return (
              <div className="rounded-md border border-border-button overflow-hidden">
                {field.options?.map((opt, index) => {
                  const isChecked = currentValues.includes(opt.value);
                  const switchId = `${field.name}-${opt.value}`;
                  return (
                    <div
                      key={opt.value}
                      className={`flex items-center justify-between gap-3 px-3 py-2.5 ${
                        index % 2 === 1 ? 'bg-bg-card' : ''
                      }`}
                    >
                      <Label
                        htmlFor={switchId}
                        className="text-sm font-normal cursor-pointer"
                      >
                        {opt.label}
                      </Label>
                      <Switch
                        id={switchId}
                        checked={isChecked}
                        disabled={field.disabled}
                        onCheckedChange={(checked) => {
                          if (field.disabled) return;
                          const next = checked
                            ? [...currentValues, opt.value]
                            : currentValues.filter((v) => v !== opt.value);
                          fieldProps.onChange(next);
                        }}
                      />
                    </div>
                  );
                })}
              </div>
            );
          },
        };
      }

      const typeMap = {
        text: FormFieldType.Text,
        number: FormFieldType.Number,
        'multi-select': FormFieldType.MultiSelect,
      } as const;

      return {
        name: field.name,
        label: field.label,
        type: typeMap[field.type as 'text' | 'number' | 'multi-select'],
        required: field.required,
        defaultValue,
        disabled: field.disabled,
        options: field.options,
        placeholder: field.label,
        ...(field.min !== undefined
          ? {
              validation: {
                min: field.min,
                message: t('modelMaxTokensMinMessage'),
              },
            }
          : {}),
        ...(field.name === 'name'
          ? {
              customValidate: (value: unknown) => {
                if (
                  typeof value === 'string' &&
                  value &&
                  existingNames.includes(value)
                ) {
                  return t('modelNameDuplicate');
                }
                return true;
              },
            }
          : {}),
      };
    });
  }, [fields, t, existingNames]);

  const handleSubmit = useCallback(
    (values: FormValues) => {
      const features = values.features;
      const featuresArray = Array.isArray(features)
        ? (features as string[])
        : [];
      // Use the caller-supplied `providerFeatureKeys` whitelist to
      // separate provider-specific feature flags from standard ones.
      // Provider-specific flags are stored as boolean values in `extra`
      // rather than as array entries in `features`, so each provider's
      // payload stays self-describing. When no whitelist is supplied,
      // all features stay in `features` (backward compatible).
      const providerSet = new Set(providerFeatureKeys ?? []);
      const standardFeatures = featuresArray.filter(
        (f) => typeof f === 'string' && !providerSet.has(f),
      );
      const providerFeatures = featuresArray.filter(
        (f) => typeof f === 'string' && providerSet.has(f),
      );
      const item: IProviderModelItem = {
        name: (values.name as string) ?? '',
        max_tokens: (values.max_tokens as number) ?? 0,
        model_types: (values.model_types as string[]) ?? [],
        features: standardFeatures.length > 0 ? standardFeatures : null,
      };
      // Collect provider-specific extra fields (e.g. SoMark's
      // element-format selects) that are not part of the standard
      // IProviderModelItem shape.
      const standardKeys = new Set([
        'name',
        'model_types',
        'max_tokens',
        'features',
      ]);
      const extra: Record<string, any> = {};
      for (const [key, value] of Object.entries(values)) {
        if (
          !standardKeys.has(key) &&
          !key.startsWith('__section_') &&
          value !== undefined
        ) {
          extra[key] = value;
        }
      }
      // Convert ALL provider-specific features to boolean extra values.
      // Selected features get `true`, unselected get `false` so the
      // backend clears toggles the user turned off (not just sets the
      // ones that are on).
      const selectedSet = new Set(providerFeatures);
      for (const key of providerFeatureKeys ?? []) {
        extra[key] = selectedSet.has(key);
      }
      if (Object.keys(extra).length > 0) {
        item.extra = extra;
      }
      onSubmit(item);
    },
    [onSubmit, providerFeatureKeys],
  );

  // Reset form whenever the dialog opens/closes, applying defaultValues for edit mode.
  useEffect(() => {
    if (!open) {
      formRef.current?.reset();
    } else if (defaultValues) {
      formRef.current?.reset(defaultValues);
    }
  }, [open, defaultValues]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl" onClick={(e) => e.stopPropagation()}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className=" max-h-[70vh] overflow-y-auto mb-12">
          <DynamicForm.Root
            ref={formRef}
            fields={dynamicFields}
            onSubmit={handleSubmit}
            defaultValues={defaultValues}
            className="pr-2 pb-2"
          >
            <DialogFooter className="absolute bottom-5 right-5 left-0 mb-0 pb-0">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={loading}
              >
                {cancelText ?? t('cancel')}
              </Button>
              <Button type="submit" disabled={loading}>
                {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {submitText ?? commonT('confirm')}
              </Button>
            </DialogFooter>
          </DynamicForm.Root>
        </div>
      </DialogContent>
    </Dialog>
  );
};

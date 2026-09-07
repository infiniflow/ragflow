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
  type: 'text' | 'number' | 'multi-select' | 'switch-group';
  /** Options for multi-select / switch-group types */
  options?: { label: string; value: string }[];
  /** Whether the field is required */
  required?: boolean;
  /** Default value */
  defaultValue?: unknown;
  /** Minimum value for number type */
  min?: number;
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
}: AddCustomModelDialogProps) => {
  const { t } = useTranslate('setting');
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
        (field.type === 'number' ? 0 : isArrayType ? [] : '');

      if (field.type === 'switch-group') {
        return {
          name: field.name,
          label: field.label,
          type: FormFieldType.Custom,
          required: field.required,
          defaultValue,
          schema: field.required
            ? z.array(z.string()).min(1, t('modelTypeRequired'))
            : z.array(z.string()).optional(),
          render: (fieldProps) => {
            const currentValues = (fieldProps.value as string[]) ?? [];
            return (
              <div className="space-y-2 rounded-md border border-border-button p-3">
                {field.options?.map((opt) => {
                  const isChecked = currentValues.includes(opt.value);
                  const switchId = `${field.name}-${opt.value}`;
                  return (
                    <div
                      key={opt.value}
                      className="flex items-center justify-between gap-3"
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
                        onCheckedChange={(checked) => {
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
      const item: IProviderModelItem = {
        name: (values.name as string) ?? '',
        max_tokens: (values.max_tokens as number) ?? 0,
        model_types: (values.model_types as string[]) ?? [],
        features:
          Array.isArray(features) && features.length > 0
            ? (features as string[])
            : null,
      };
      onSubmit(item);
    },
    [onSubmit],
  );

  // Reset form whenever the dialog closes, so the next open starts fresh.
  useEffect(() => {
    if (!open) {
      formRef.current?.reset();
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" onClick={(e) => e.stopPropagation()}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>

        <DynamicForm.Root
          ref={formRef}
          fields={dynamicFields}
          onSubmit={handleSubmit}
        >
          <DialogFooter>
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
              {submitText ?? t('confirm')}
            </Button>
          </DialogFooter>
        </DynamicForm.Root>
      </DialogContent>
    </Dialog>
  );
};

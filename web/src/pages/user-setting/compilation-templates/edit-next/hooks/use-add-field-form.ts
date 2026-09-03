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

import { ICompilationTemplateSection } from '@/interfaces/database/compilation-template';
import {
  createEmptyField,
  getAvailableTypeOptions,
  getFieldKeyOrder,
  getRequiredFieldKeys,
} from '../utils';
import { useCallback, useEffect, useMemo } from 'react';
import { useForm } from 'react-hook-form';

type UseAddFieldFormOptions = {
  open: boolean;
  builtinSection?: ICompilationTemplateSection;
  existingFields?: Record<string, string>[];
  initialField?: Record<string, string>;
};

export const useAddFieldForm = ({
  open,
  builtinSection,
  existingFields,
  initialField,
}: UseAddFieldFormOptions) => {
  const form = useForm<Record<string, string>>({
    defaultValues: {},
  });

  const fieldKeys = useMemo(() => {
    const firstField = builtinSection?.fields?.[0];
    const keys = firstField
      ? Object.keys(firstField)
      : ['type', 'description', 'rule'];
    return getFieldKeyOrder(keys);
  }, [builtinSection]);

  const hasTypeField = fieldKeys.includes('type');

  const requiredKeys = useMemo(
    () => getRequiredFieldKeys(fieldKeys),
    [fieldKeys],
  );

  const typeOptions = useMemo(
    () =>
      getAvailableTypeOptions(
        builtinSection,
        existingFields,
        initialField?.type,
      ),
    [builtinSection, existingFields, initialField],
  );

  const buildField = useCallback(
    (typeValue: string) => {
      const matched = builtinSection?.fields?.find(
        (field) => field.type === typeValue,
      );
      if (matched) {
        const normalized: Record<string, string> = {};
        fieldKeys.forEach((key) => {
          normalized[key] = (matched as Record<string, string>)[key] ?? '';
        });
        return normalized;
      }
      const empty = createEmptyField(fieldKeys);
      if (hasTypeField) {
        empty.type = typeValue;
      }
      return empty;
    },
    [builtinSection, fieldKeys, hasTypeField],
  );

  useEffect(() => {
    if (!open) return;

    if (initialField) {
      const normalized: Record<string, string> = {};
      fieldKeys.forEach((key) => {
        normalized[key] = initialField[key] ?? '';
      });
      form.reset(normalized);
      return;
    }

    form.reset(createEmptyField(fieldKeys));
  }, [fieldKeys, form, initialField, open]);

  const handleTypeChange = useCallback(
    (value: string) => {
      if (initialField) {
        form.setValue('type', value, {
          shouldDirty: true,
          shouldTouch: true,
        });
        return;
      }

      const nextField = buildField(value);
      Object.entries(nextField).forEach(([key, val]) => {
        form.setValue(key, val, {
          shouldDirty: true,
          shouldTouch: true,
        });
        if (val) form.clearErrors(key);
      });
    },
    [buildField, form, initialField],
  );

  const handleSubmit = useCallback(
    (onAdd: (field: Record<string, string>) => void) => {
      return form.handleSubmit((values) => {
        onAdd(values);
      });
    },
    [form],
  );

  return {
    form,
    fieldKeys,
    hasTypeField,
    requiredKeys,
    typeOptions,
    handleTypeChange,
    handleSubmit,
  };
};

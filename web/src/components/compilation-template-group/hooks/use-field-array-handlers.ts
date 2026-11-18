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

import { useCallback } from 'react';
import { ArrayPath, UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '../schema';

export const useFieldArrayHandlers = (
  form: UseFormReturn<FormSchemaType>,
  activeFieldsPath: ArrayPath<FormSchemaType>,
  editingFieldIndex: number | null,
  setEditingFieldIndex: (index: number | null) => void,
) => {
  const handleAddField = useCallback(
    (field: Record<string, string>) => {
      const currentFields =
        (form.getValues(activeFieldsPath) as
          | Record<string, string>[]
          | undefined) ?? [];
      if (editingFieldIndex !== null) {
        const newFields = [...currentFields];
        newFields[editingFieldIndex] = field;
        form.setValue(activeFieldsPath, newFields, {
          shouldValidate: false,
          shouldDirty: true,
          shouldTouch: true,
        });
      } else {
        form.setValue(activeFieldsPath, [...currentFields, field], {
          shouldValidate: false,
          shouldDirty: true,
          shouldTouch: true,
        });
      }
      setEditingFieldIndex(null);
    },
    [activeFieldsPath, editingFieldIndex, form, setEditingFieldIndex],
  );

  return { handleAddField };
};

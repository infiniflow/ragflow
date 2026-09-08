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

import { useMemo } from 'react';
import { ArrayPath, UseFormReturn, useWatch } from 'react-hook-form';

import {
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { FormSchemaType } from '../schema';

export const useTemplateSectionData = (
  form: UseFormReturn<FormSchemaType>,
  selectedTemplateIndex: number,
  activeSectionTab: string,
  builtinTemplate: ICompilationTemplateBuiltin | undefined,
  editingFieldIndex: number | null,
) => {
  const activeSectionPath = `templates.${selectedTemplateIndex}.config.${activeSectionTab}`;
  const activeFieldsPath =
    `${activeSectionPath}.fields` as ArrayPath<FormSchemaType>;

  const builtinSection = useMemo(() => {
    return builtinTemplate?.config?.[activeSectionTab] as
      | ICompilationTemplateSection
      | undefined;
  }, [activeSectionTab, builtinTemplate?.config]);

  const existingFields = useWatch({
    control: form.control,
    name: activeFieldsPath,
  }) as Record<string, string>[] | undefined;

  const editingField = useMemo(() => {
    if (editingFieldIndex === null) return undefined;
    return ((form.getValues(activeFieldsPath) as
      | Record<string, string>[]
      | undefined) ?? [])[editingFieldIndex];
  }, [activeFieldsPath, editingFieldIndex, form]);

  return {
    activeSectionPath,
    activeFieldsPath,
    builtinSection,
    existingFields,
    editingField,
  };
};

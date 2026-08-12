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

import { CompilationTemplateKind } from '@/constants/compilation';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { UseFormReturn, useWatch } from 'react-hook-form';

export const useTemplateAddButton = (
  form: UseFormReturn<FormSchemaType>,
  kindOptions: { label: string; value: string }[],
) => {
  const templates =
    useWatch({ control: form.control, name: 'templates' }) ?? [];

  const hasTemplateWithoutKind = templates.some((template) => !template.kind);
  const hasArtifactsTemplate = templates.some(
    (template) => template.kind === CompilationTemplateKind.Artifacts,
  );
  const selectedKinds = new Set(
    templates
      .map((template) => template.kind)
      .filter((kind): kind is string => Boolean(kind)),
  );
  const allKindsSelected =
    kindOptions.length > 0 &&
    kindOptions.every((option) => selectedKinds.has(option.value));

  const isAddButtonHidden =
    hasTemplateWithoutKind || hasArtifactsTemplate || allKindsSelected;

  return { templates, isAddButtonHidden };
};

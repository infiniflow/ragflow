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
import { FormSchemaType } from '../schema';
import { useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

export const useAvailableKindOptions = (
  form: UseFormReturn<FormSchemaType>,
  kindOptions: { label: string; value: string }[],
  selectedTemplateIndex: number,
) => {
  const kind = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.kind`,
  });

  const templates = useWatch({ control: form.control, name: 'templates' });

  const availableKindOptions = useMemo(() => {
    const otherSelectedKinds = new Set(
      templates
        ?.filter((_, index) => index !== selectedTemplateIndex)
        .map((template) => template.kind)
        .filter((value): value is string => Boolean(value)) ?? [],
    );

    const hasOtherNonArtifactsKind = Array.from(otherSelectedKinds).some(
      (value) => value !== CompilationTemplateKind.Artifacts,
    );
    const hasOtherArtifactsKind = otherSelectedKinds.has(
      CompilationTemplateKind.Artifacts,
    );

    return kindOptions.filter((option) => {
      if (option.value === kind) return true;
      if (otherSelectedKinds.has(option.value)) return false;
      if (
        hasOtherNonArtifactsKind &&
        option.value === CompilationTemplateKind.Artifacts
      ) {
        return false;
      }
      if (
        hasOtherArtifactsKind &&
        option.value !== CompilationTemplateKind.Artifacts
      ) {
        return false;
      }
      return true;
    });
  }, [kindOptions, kind, selectedTemplateIndex, templates]);

  return availableKindOptions;
};

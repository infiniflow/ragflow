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

import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '../schema';
import { buildConfigFromBuiltin } from '../utils';

type FieldLike = {
  value: string;
  onChange: (value: string) => void;
};

type UseTemplateKindChangeOptions = {
  form: UseFormReturn<FormSchemaType>;
  index: number;
  builtins: ICompilationTemplateBuiltin[];
};

export const useTemplateKindChange = ({
  form,
  index,
  builtins,
}: UseTemplateKindChangeOptions) => {
  return (field: FieldLike, value: string) => {
    if (value && value !== field.value) {
      const builtinTemplate = builtins.find(
        (template) => template.kind === value,
      );
      if (builtinTemplate) {
        form.setValue(
          `templates.${index}.config`,
          buildConfigFromBuiltin(builtinTemplate, value),
          { shouldValidate: false },
        );
      }
    }
    field.onChange(value);
  };
};

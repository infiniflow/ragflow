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

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';

import { buildFormSchema, FormSchemaType } from '../schema';
import { DefaultValues } from '../constant';
import { generateTemplateName, transformGroupDetailToForm } from '../utils';

type UseCompilationTemplateGroupFormOptions = {
  detail?: ICompilationTemplateGroup;
  defaultLlmId?: string;
  isCreate: boolean;
};

export const useCompilationTemplateGroupForm = ({
  detail,
  defaultLlmId,
  isCreate,
}: UseCompilationTemplateGroupFormOptions) => {
  const { t } = useTranslation();

  const defaultValues = useMemo<FormSchemaType>(() => {
    if (!isCreate) return DefaultValues;
    return {
      ...DefaultValues,
      templates: [
        {
          ...DefaultValues.templates[0],
          name: generateTemplateName(),
        },
      ],
    };
  }, [isCreate]);

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(buildFormSchema(t)),
    defaultValues,
    mode: 'onChange',
  });

  useEffect(() => {
    if (detail) {
      form.reset(transformGroupDetailToForm(detail));
    } else if (
      isCreate &&
      defaultLlmId &&
      !form.getValues('templates.0.llm_id')
    ) {
      form.setValue('templates.0.llm_id', defaultLlmId);
    }
  }, [defaultLlmId, detail, form, isCreate]);

  return { form };
};

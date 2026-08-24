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

import {
  useCreateCompilationTemplateGroup,
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { formatKindLabel } from '@/utils/compilation-template-util';

import { useCompilationTemplateGroupForm } from './use-compilation-template-group-form';
import { useCompilationTemplateGroupSubmit } from './use-compilation-template-group-submit';

type UseEditNextCompilationTemplateGroupOptions = {
  onSuccess: () => void;
};

export const useEditNextCompilationTemplateGroup = ({
  onSuccess,
}: UseEditNextCompilationTemplateGroupOptions) => {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();

  const isCreate = isCreateCompilationTemplateGroup(id);

  const { data: detail } = useFetchCompilationTemplateGroup();
  const { data: builtins, kindOptions: builtinKindOptions } =
    useFetchBuiltinCompilationTemplates();

  const { createGroup, loading: createLoading } =
    useCreateCompilationTemplateGroup();
  const { updateGroup, loading: updateLoading } =
    useUpdateCompilationTemplateGroup();

  const kindOptions = useMemo(
    () =>
      builtinKindOptions.map((option) => ({
        ...option,
        label: formatKindLabel(t, option.value),
      })),
    [builtinKindOptions, t],
  );

  const { form } = useCompilationTemplateGroupForm({ detail });

  const { onSubmit } = useCompilationTemplateGroupSubmit({
    isCreate,
    id,
    createGroup,
    updateGroup,
    onSuccess,
  });

  return {
    id,
    form,
    kindOptions,
    builtins,
    onSubmit,
    isCreate,
    isLoading: isCreate ? createLoading : updateLoading,
  };
};

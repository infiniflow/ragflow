import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useCreateCompilationTemplateGroup,
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { useMemo } from 'react';
import { useParams } from 'react-router';

import { formatKindLabel } from '@/utils/compilation-template-util';

import { useCompilationTemplateGroupForm } from '@/pages/user-setting/compilation-templates/create-next/hooks/use-compilation-template-group-form';
import { useCompilationTemplateGroupSubmit } from '@/pages/user-setting/compilation-templates/create-next/hooks/use-compilation-template-group-submit';

export const useCreateNextCompilationTemplateGroup = () => {
  const { id } = useParams<{ id: string }>();
  const { navigateToCompilationTemplates } = useNavigatePage();

  const isCreate = isCreateCompilationTemplateGroup(id);

  const { data: detail } = useFetchCompilationTemplateGroup();
  const { data: builtins, kindOptions: builtinKindOptions } =
    useFetchBuiltinCompilationTemplates();
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const { createGroup, loading: createLoading } =
    useCreateCompilationTemplateGroup();
  const { updateGroup, loading: updateLoading } =
    useUpdateCompilationTemplateGroup();

  const kindOptions = useMemo(
    () =>
      builtinKindOptions.map((option) => ({
        ...option,
        label: formatKindLabel(option.value),
      })),
    [builtinKindOptions],
  );

  const { form } = useCompilationTemplateGroupForm({
    detail,
    defaultLlmId: defaultModelDictionary.llm_id,
    isCreate,
  });

  const { onSubmit } = useCompilationTemplateGroupSubmit({
    isCreate,
    id,
    createGroup,
    updateGroup,
    onSuccess: navigateToCompilationTemplates,
  });

  return {
    id,
    form,
    kindOptions,
    builtins,
    onSubmit,
    isCreate,
    isLoading: isCreate ? createLoading : updateLoading,
    navigateToCompilationTemplates,
  };
};

import {
  useCreateCompilationTemplateGroup,
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useMemo } from 'react';
import { useParams } from 'react-router';

import { formatKindLabel } from '@/utils/compilation-template-util';

export const useCompilationTemplateGroupData = () => {
  const { id } = useParams<{ id: string }>();
  const isCreate = id === 'create';

  const { data: detail } = useFetchCompilationTemplateGroup(id);
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

  const isLoading = createLoading || updateLoading;

  return {
    id,
    isCreate,
    detail,
    builtins,
    kindOptions,
    defaultModelDictionary,
    createGroup,
    updateGroup,
    isLoading,
  };
};

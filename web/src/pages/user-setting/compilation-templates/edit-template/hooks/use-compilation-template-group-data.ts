import {
  useCreateCompilationTemplateGroup,
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

export const useCompilationTemplateGroupData = () => {
  const { id } = useParams<{ id: string }>();
  const isCreate = id === 'create';
  const { t } = useTranslation();

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
        label: t(`knowledgeCompilation.kind.${option.value}`),
      })),
    [builtinKindOptions, t],
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

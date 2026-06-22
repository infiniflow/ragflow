import {
  useCreateCompilationTemplate,
  useFetchBuiltinCompilationTemplates,
  useFetchCompilationTemplate,
  useUpdateCompilationTemplate,
} from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

export const useCompilationTemplateData = () => {
  const { id } = useParams<{ id: string }>();
  const isCreate = id === 'create';
  const { t } = useTranslation();

  const { data: detail } = useFetchCompilationTemplate(id);
  const {
    data: builtins,
    typeOptions,
    kindOptions: builtinKindOptions,
  } = useFetchBuiltinCompilationTemplates();
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const { createTemplate, loading: createLoading } =
    useCreateCompilationTemplate();
  const { updateTemplate, loading: updateLoading } =
    useUpdateCompilationTemplate();

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
    typeOptions,
    kindOptions,
    defaultModelDictionary,
    createTemplate,
    updateTemplate,
    isLoading,
  };
};

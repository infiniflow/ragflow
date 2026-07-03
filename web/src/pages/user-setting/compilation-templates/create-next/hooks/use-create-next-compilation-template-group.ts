import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useCreateCompilationTemplateGroup,
  useFetchCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { useCompilationTemplateGroupForm } from '@/pages/user-setting/compilation-templates/edit-template/hooks/use-compilation-template-group-form';
import { useCompilationTemplateGroupSubmit } from '@/pages/user-setting/compilation-templates/edit-template/hooks/use-compilation-template-group-submit';

export const useCreateNextCompilationTemplateGroup = () => {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { navigateToCompilationTemplates } = useNavigatePage();

  const { data: detail } = useFetchCompilationTemplateGroup(id);
  const { data: builtins, kindOptions: builtinKindOptions } =
    useFetchBuiltinCompilationTemplates();
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const { createGroup, loading: createLoading } =
    useCreateCompilationTemplateGroup();

  const kindOptions = useMemo(
    () =>
      builtinKindOptions.map((option) => ({
        ...option,
        label: t(`knowledgeCompilation.kind.${option.value}`),
      })),
    [builtinKindOptions, t],
  );

  const { form } = useCompilationTemplateGroupForm({
    detail,
    defaultLlmId: defaultModelDictionary.llm_id,
    isCreate: true,
  });

  const { onSubmit } = useCompilationTemplateGroupSubmit({
    isCreate: true,
    id,
    createGroup,
    updateGroup: async () =>
      ({ code: -1 }) as { code: number } & Record<string, unknown>,
    onSuccess: navigateToCompilationTemplates,
  });

  return {
    id,
    form,
    kindOptions,
    builtins,
    onSubmit,
    isLoading: createLoading,
    navigateToCompilationTemplates,
  };
};

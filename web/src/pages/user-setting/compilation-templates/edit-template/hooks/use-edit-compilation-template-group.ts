import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useWatch } from 'react-hook-form';

import { DefaultValues } from '../utils';
import { useCompilationTemplateGroupData } from './use-compilation-template-group-data';
import { useCompilationTemplateGroupForm } from './use-compilation-template-group-form';
import { useCompilationTemplateGroupSubmit } from './use-compilation-template-group-submit';

export const useEditCompilationTemplateGroup = () => {
  const {
    id,
    isCreate,
    detail,
    builtins,
    kindOptions,
    defaultModelDictionary,
    createGroup,
    updateGroup,
    isLoading,
  } = useCompilationTemplateGroupData();

  const { navigateToCompilationTemplates } = useNavigatePage();

  const { form } = useCompilationTemplateGroupForm({
    detail,
    defaultLlmId: defaultModelDictionary.llm_id,
    isCreate,
  });

  const watchedValues = useWatch({ control: form.control }) ?? DefaultValues;

  const { onSubmit } = useCompilationTemplateGroupSubmit({
    isCreate,
    id,
    createGroup,
    updateGroup,
    onSuccess: navigateToCompilationTemplates,
  });

  return {
    id,
    isCreate,
    form,
    watchedValues,
    kindOptions,
    builtins,
    onSubmit,
    isLoading,
    navigateToCompilationTemplates,
  };
};

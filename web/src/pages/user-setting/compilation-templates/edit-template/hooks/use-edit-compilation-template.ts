import { useMemo } from 'react';
import { useWatch } from 'react-hook-form';

import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';

import { DefaultValues, isConfigMetaKey } from '../utils';
import { useCompilationTemplateData } from './use-compilation-template-data';
import { useCompilationTemplateForm } from './use-compilation-template-form';
import { useCompilationTemplateSubmit } from './use-compilation-template-submit';

export const useEditCompilationTemplate = () => {
  const {
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
  } = useCompilationTemplateData();

  const { navigateToCompilationTemplates } = useNavigatePage();

  const { form, builtinTemplate } = useCompilationTemplateForm({
    detail,
    defaultLlmId: defaultModelDictionary.llm_id,
    isCreate,
    builtins,
  });

  const watchedValues = useWatch({ control: form.control }) ?? DefaultValues;

  const sectionNames = useMemo(() => {
    return Object.keys(builtinTemplate?.config ?? {}).filter(
      (key) => !isConfigMetaKey(key),
    );
  }, [builtinTemplate]);

  const { onSubmit } = useCompilationTemplateSubmit({
    isCreate,
    id,
    createTemplate,
    updateTemplate,
    onSuccess: navigateToCompilationTemplates,
  });

  return {
    id,
    isCreate,
    form,
    watchedValues,
    kindOptions,
    sectionNames,
    builtinTemplate,
    typeOptions,
    onSubmit,
    isLoading,
    navigateToCompilationTemplates,
  };
};

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';

import { buildFormSchema, FormSchemaType } from '../schema';
import { DefaultValues, transformGroupDetailToForm } from '../utils';

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

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(buildFormSchema(t)),
    defaultValues: DefaultValues,
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

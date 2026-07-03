import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo } from 'react';
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

  const defaultValues = useMemo<FormSchemaType>(() => {
    if (!isCreate) return DefaultValues;
    return {
      ...DefaultValues,
      templates: [
        {
          ...DefaultValues.templates[0],
          name: `${t('setting.template')} #1`,
        },
      ],
    };
  }, [isCreate, t]);

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(buildFormSchema(t)),
    defaultValues,
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

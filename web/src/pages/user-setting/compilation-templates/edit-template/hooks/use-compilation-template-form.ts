import {
  ICompilationTemplate,
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo, useRef } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { buildFormSchema, FormSchemaType } from '../schema';
import {
  DefaultValues,
  isConfigMetaKey,
  normalizeSection,
  transformDetailToForm,
} from '../utils';

type UseCompilationTemplateFormOptions = {
  detail?: ICompilationTemplate;
  defaultLlmId?: string;
  isCreate: boolean;
  builtins: ICompilationTemplateBuiltin[];
};

export const useCompilationTemplateForm = ({
  detail,
  defaultLlmId,
  isCreate,
  builtins,
}: UseCompilationTemplateFormOptions) => {
  const { t } = useTranslation();

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(buildFormSchema(t)),
    defaultValues: DefaultValues,
  });

  useEffect(() => {
    if (detail) {
      form.reset(transformDetailToForm(detail));
    } else if (isCreate && defaultLlmId && !form.getValues('llm_id')) {
      form.setValue('llm_id', defaultLlmId);
    }
  }, [defaultLlmId, detail, form, isCreate]);

  const kind = useWatch({ control: form.control, name: 'kind' });

  const builtinTemplate = useMemo(
    () => builtins.find((template) => template.kind === kind),
    [builtins, kind],
  );

  const previousKindRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!kind || !builtinTemplate) return;

    // In edit mode, the first kind change comes from detail initialization; skip it to avoid overwriting the existing config.
    if (!isCreate && previousKindRef.current === undefined) {
      previousKindRef.current = kind;
      return;
    }

    if (previousKindRef.current === kind) return;
    previousKindRef.current = kind;

    const sections: FormSchemaType['config'] = {
      kind,
      llm_id: form.getValues('llm_id'),
      global_rules: builtinTemplate.config.global_rules ?? '',
    };

    Object.entries(builtinTemplate.config).forEach(([key, value]) => {
      if (isConfigMetaKey(key)) return;
      sections[key] = normalizeSection(
        value as ICompilationTemplateSection,
      ) as FormSchemaType['config'][string];
    });

    const oldConfig = form.getValues('config') ?? {};

    form.reset({
      name: form.getValues('name'),
      description: form.getValues('description'),
      llm_id: form.getValues('llm_id'),
      kind,
      config: sections,
    });

    Object.keys(oldConfig).forEach((key) => {
      if (!isConfigMetaKey(key) && !(key in sections)) {
        form.unregister(`config.${key}` as `config.${string}`);
      }
    });
  }, [builtinTemplate, form, isCreate, kind]);

  return { form, kind, builtinTemplate };
};

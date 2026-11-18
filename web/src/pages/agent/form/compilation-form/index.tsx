import { CompilationTemplateFormField } from '@/components/compilation-template-form-field';
import {
  LlmSettingFieldItems,
  LlmSettingSchema,
} from '@/components/llm-setting-items/next';
import { useSyncExternalFormErrors } from '@/components/pipeline-operator-tabs/use-sync-external-form-errors';
import { Form } from '@/components/ui/form';
import {
  useRevalidateUnavailableValue,
  useUnavailableCompilationTemplateGroupFormSchema,
  useUnavailableModelFormSchema,
} from '@/hooks/use-unavailable-value-validation';
import { zodResolver } from '@hookform/resolvers/zod';
import type { TFunction } from 'i18next';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialCompilationValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

export function buildCompilationFormSchema(t: TFunction) {
  return z.object({
    compilation_template_group_id: z
      .string()
      .min(1, t('knowledgeConfiguration.compilationTemplateRequired')),
    ...LlmSettingSchema,
  });
}

function useFormSchema() {
  const { t } = useTranslation();
  return buildCompilationFormSchema(t);
}

export type CompilationFormSchemaType = z.infer<
  ReturnType<typeof useFormSchema>
>;

const outputList = buildOutputList(initialCompilationValues.outputs);

const CompilationForm = ({
  node,
  onValuesChange,
  hideOutputs,
  externalErrors,
}: INextOperatorForm) => {
  const defaultValues = useFormValues(initialCompilationValues, node);
  const ownerTenantId = useOwnerTenantId();
  const FormSchema = useFormSchema();
  const { formSchema: groupFormSchema, templateGroupsFetched } =
    useUnavailableCompilationTemplateGroupFormSchema(FormSchema, {
      ownerTenantId,
    });
  const { formSchema, modelsFetched } = useUnavailableModelFormSchema(
    groupFormSchema,
    { ownerTenantId },
  );

  const form = useForm<CompilationFormSchemaType>({
    defaultValues,
    resolver: zodResolver(formSchema),
    mode: 'onChange',
  });

  useSyncExternalFormErrors(form, externalErrors);

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  // Persisted model/group references from an imported dsl.json do not exist
  // under the importer's tenant — surface the errors once the lists load.
  useRevalidateUnavailableValue(
    form,
    templateGroupsFetched,
    'compilation_template_group_id',
  );
  useRevalidateUnavailableValue(form, modelsFetched, 'llm_id');

  return (
    <Form {...form}>
      <FormWrapper>
        <CompilationTemplateFormField
          name="compilation_template_group_id"
          ownerTenantId={ownerTenantId}
        ></CompilationTemplateFormField>
        <LlmSettingFieldItems
          ownerTenantId={ownerTenantId}
        ></LlmSettingFieldItems>
      </FormWrapper>
      {!hideOutputs && (
        <div className="p-5">
          <Output list={outputList}></Output>
        </div>
      )}
    </Form>
  );
};

export default memo(CompilationForm);

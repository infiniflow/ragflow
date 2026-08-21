import { CompilationTemplateFormField } from '@/components/compilation-template-form-field';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import {
  getBackendLanguage,
  subscribeBackendLanguage,
} from '@/utils/backend-runtime';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useSyncExternalStore } from 'react';
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

function useFormSchema() {
  const { t } = useTranslation();
  const FormSchema = z.object({
    compilation_template_group_id: z
      .string()
      .min(1, t('knowledgeConfiguration.compilationTemplateRequired')),
    llm_id: z.string().optional(),
    mode: z.enum(['entity', 'topic']),
  });

  return FormSchema;
}

export type CompilationFormSchemaType = z.infer<
  ReturnType<typeof useFormSchema>
>;

const outputList = buildOutputList(initialCompilationValues.outputs);

const CompilationForm = ({
  node,
  onValuesChange,
  hideOutputs,
}: INextOperatorForm) => {
  const defaultValues = useFormValues(initialCompilationValues, node);
  const ownerTenantId = useOwnerTenantId();
  const { t } = useTranslation();
  const FormSchema = useFormSchema();
  const backendLanguage = useSyncExternalStore(
    subscribeBackendLanguage,
    getBackendLanguage,
    getBackendLanguage,
  );

  const form = useForm<CompilationFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  return (
    <Form {...form}>
      <FormWrapper>
        <CompilationTemplateFormField name="compilation_template_group_id"></CompilationTemplateFormField>
        <LargeModelFormField
          name="llm_id"
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>
        {backendLanguage === 'go' && (
          <RAGFlowFormItem
            name="mode"
            label={t('knowledgeCompilation.wikiMode')}
            tooltip={t('knowledgeCompilation.wikiModeTip')}
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={[
                  { label: t('knowledgeCompilation.entityMode'), value: 'entity' },
                  { label: t('knowledgeCompilation.topicMode'), value: 'topic' },
                ]}
              />
            )}
          </RAGFlowFormItem>
        )}
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

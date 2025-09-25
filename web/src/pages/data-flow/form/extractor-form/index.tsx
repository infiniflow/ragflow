import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { PromptEditor } from '@/pages/agent/form/components/prompt-editor';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ContextGeneratorFieldName,
  initialContextValues,
} from '../../constant';
import { useBuildNodeOutputOptions } from '../../hooks/use-build-options';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';

export const FormSchema = z.object({
  sys_prompt: z.string(),
  prompts: z.string().optional(),
  ...LlmSettingSchema,
  field_name: z.array(z.string()),
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

const ExtractorForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialContextValues, node);
  const { t } = useTranslation();

  const form = useForm<ExtractorFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const promptOptions = useBuildNodeOutputOptions(node?.id);

  const options = buildOptions(ContextGeneratorFieldName, t, 'dataflow');

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField></LargeModelFormField>
        <RAGFlowFormItem label={t('dataflow.fieldName')} name="field_name">
          {(field) => (
            <SelectWithSearch
              {...field}
              placeholder={t('dataFlowPlaceholder')}
              options={options}
            ></SelectWithSearch>
          )}
        </RAGFlowFormItem>
        <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
          <PromptEditor
            placeholder={t('flow.messagePlaceholder')}
            showToolbar={true}
            baseOptions={promptOptions}
          ></PromptEditor>
        </RAGFlowFormItem>
        <RAGFlowFormItem label={t('flow.userPrompt')} name="prompts">
          <PromptEditor
            showToolbar={true}
            baseOptions={promptOptions}
          ></PromptEditor>
        </RAGFlowFormItem>
      </FormWrapper>
    </Form>
  );
};

export default memo(ExtractorForm);

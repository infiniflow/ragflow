import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { useBuildPromptExtraPromptOptions } from '@/pages/agent/form/agent-form/use-build-prompt-options';
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
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialContextValues.outputs);

export const FormSchema = z.object({
  sys_prompt: z.string(),
  prompts: z.string().optional(),
  ...LlmSettingSchema,
  field_name: z.array(z.string()),
});

export type ContextFormSchemaType = z.infer<typeof FormSchema>;

const ContextForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialContextValues, node);
  const { t } = useTranslation();

  const form = useForm<ContextFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const { edges } = useGraphStore((state) => state);

  const { extraOptions } = useBuildPromptExtraPromptOptions(edges, node?.id);

  const options = buildOptions(ContextGeneratorFieldName, t, 'dataflow');

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField></LargeModelFormField>
        <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
          <PromptEditor
            placeholder={t('flow.messagePlaceholder')}
            showToolbar={true}
            extraOptions={extraOptions}
          ></PromptEditor>
        </RAGFlowFormItem>
        <RAGFlowFormItem label={t('flow.userPrompt')} name="prompts">
          <PromptEditor showToolbar={true}></PromptEditor>
        </RAGFlowFormItem>
        <RAGFlowFormItem label={t('dataflow.fieldName')} name="field_name">
          {(field) => (
            <MultiSelect
              onValueChange={field.onChange}
              placeholder={t('dataFlowPlaceholder')}
              defaultValue={field.value}
              options={options}
            ></MultiSelect>
          )}
        </RAGFlowFormItem>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(ContextForm);

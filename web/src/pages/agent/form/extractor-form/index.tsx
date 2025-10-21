import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
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
  initialExtractorValues,
} from '../../constant/pipeline';
import { useBuildNodeOutputOptions } from '../../hooks/use-build-options';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { useSwitchPrompt } from './use-switch-prompt';

export const FormSchema = z.object({
  field_name: z.string(),
  sys_prompt: z.string(),
  prompts: z.string().optional(),
  ...LlmSettingSchema,
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialExtractorValues.outputs);

const ExtractorForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialExtractorValues, node);
  const { t } = useTranslation();

  const form = useForm<ExtractorFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    // mode: 'onChange',
  });

  const promptOptions = useBuildNodeOutputOptions(node?.id);

  const options = buildOptions(ContextGeneratorFieldName, t, 'flow');

  const {
    handleFieldNameChange,
    confirmSwitch,
    hideModal,
    visible,
    cancelSwitch,
  } = useSwitchPrompt(form);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField></LargeModelFormField>
        <RAGFlowFormItem label={t('flow.fieldName')} name="field_name">
          {(field) => (
            <SelectWithSearch
              onChange={(value) => {
                field.onChange(value);
                handleFieldNameChange(value);
              }}
              value={field.value}
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
        <Output list={outputList}></Output>
      </FormWrapper>
      {visible && (
        <ConfirmDeleteDialog
          title={t('flow.switchPromptMessage')}
          open
          onOpenChange={hideModal}
          onOk={confirmSwitch}
          onCancel={cancelSwitch}
        ></ConfirmDeleteDialog>
      )}
    </Form>
  );
};

export default memo(ExtractorForm);

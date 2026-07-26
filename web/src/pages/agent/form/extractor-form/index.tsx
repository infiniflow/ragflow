import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { AsyncTreeSelect } from '@/components/ui/async-tree-select';
import { Form } from '@/components/ui/form';
import { PromptEditor } from '@/pages/agent/form/components/prompt-editor';
import { isGoBackend } from '@/utils/backend-runtime';
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
import { useOwnerTenantId } from '../../context';
import { useBuildNodeOutputOptions } from '../../hooks/use-build-options';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { useSwitchPrompt } from './use-switch-prompt';
import { canSelectTagFile, useTagFileTree } from './use-tag-file-tree';
import { FormLayout } from '@/constants/form';

export const FormSchema = z.object({
  field_name: z.string(),
  sys_prompt: z.string(),
  prompts: z.string().optional(),
  auto_keywords: z.number().optional(),
  auto_questions: z.number().optional(),
  auto_tags: z.number().optional(),
  tag_file_id: z.string().optional(),
  ...LlmSettingSchema,
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialExtractorValues.outputs);

const ExtractorForm = ({
  node,
  onValuesChange,
  hideOutputs,
}: INextOperatorForm) => {
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
  useFormChangeCallback(form, onValuesChange);

  const ownerTenantId = useOwnerTenantId();
  const isToc = form.getValues('field_name') === 'toc';

  const { treeData, loadData } = useTagFileTree();

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>
        <AutoKeywordsFormField name="auto_keywords"></AutoKeywordsFormField>
        <AutoQuestionsFormField name="auto_questions"></AutoQuestionsFormField>
        {isGoBackend() && (
          <>
            <SliderInputFormField
              name="auto_tags"
              label={t('knowledgeDetails.autoTags')}
              min={1}
              max={10}
              defaultValue={1}
              layout={FormLayout.Vertical}
            ></SliderInputFormField>

            <RAGFlowFormItem label={t('flow.tagFile')} name="tag_file_id">
              {(field) => (
                <AsyncTreeSelect
                  treeData={treeData}
                  value={field.value}
                  onChange={field.onChange}
                  loadData={loadData}
                  canSelect={canSelectTagFile}
                ></AsyncTreeSelect>
              )}
            </RAGFlowFormItem>
          </>
        )}

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

        {!isToc && (
          <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
            <PromptEditor
              placeholder={t('flow.messagePlaceholder')}
              showToolbar={true}
              baseOptions={promptOptions}
            ></PromptEditor>
          </RAGFlowFormItem>
        )}

        <RAGFlowFormItem
          label={isToc ? t('flow.tocDataSource') : t('flow.userPrompt')}
          name="prompts"
        >
          <PromptEditor
            showToolbar={true}
            baseOptions={promptOptions}
          ></PromptEditor>
        </RAGFlowFormItem>

        {!hideOutputs && <Output list={outputList}></Output>}
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

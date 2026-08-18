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
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { PromptEditor } from '@/pages/agent/form/components/prompt-editor';
import { MetadataType } from '@/pages/dataset/components/metedata/constant';
import {
  useManageMetadata,
  util,
} from '@/pages/dataset/components/metedata/hooks/use-manage-modal';
import {
  IBuiltInMetadataItem,
  IMetaDataReturnJSONSettings,
} from '@/pages/dataset/components/metedata/interface';
import { ManageMetadataModal } from '@/pages/dataset/components/metedata/manage-modal';
import { isGoBackend } from '@/utils/backend-runtime';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Settings } from 'lucide-react';
import { memo, useCallback } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
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
  // Builtin auto-metadata (mirrors Python's Auto metadata): enable_metadata
  // toggle + metadata / built_in_metadata field schema, consumed by the Go
  // extractor's runEnableMetadata at parse time.
  enable_metadata: z.number().optional(),
  metadata: z.any().optional(),
  built_in_metadata: z.any().optional(),
  ...LlmSettingSchema,
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

// Builtin auto-extract node id hardcoded in every builtin ingestion DSL
// template (internal/ingestion/pipeline/template/*.json). Only this node
// shows the Auto metadata option; canvas / user-pipeline extractor nodes do
// not.
const BuiltinAutoExtractNodeId = 'Extractor:AutoExtractDefault';

// ExtractorAutoMetadata mirrors Python's dataset "Auto metadata" control: an
// enable_metadata switch plus a field-schema editor (custom + built-in).
// Values are stored on the node params (enable_metadata / metadata /
// built_in_metadata) and drive the Go extractor's runEnableMetadata.
function ExtractorAutoMetadata() {
  const { t } = useTranslation();
  const form = useFormContext<ExtractorFormSchemaType>();
  const {
    manageMetadataVisible,
    showManageMetadataModal,
    hideManageMetadataModal,
    tableData,
    config: metadataConfig,
  } = useManageMetadata();

  const handleOpen = useCallback(() => {
    showManageMetadataModal({
      metadata: util.metaDataSettingJSONToMetaDataTableData(
        form.getValues('metadata') || [],
      ),
      isCanAdd: true,
      type: MetadataType.Setting,
      builtInMetadata: form.getValues('built_in_metadata') || [],
    });
  }, [form, showManageMetadataModal]);

  const handleSave = useCallback(
    (data?: {
      metadata?: IMetaDataReturnJSONSettings;
      builtInMetadata?: IBuiltInMetadataItem[];
    }) => {
      form.setValue('metadata', data?.metadata || []);
      form.setValue('built_in_metadata', data?.builtInMetadata || []);
      form.setValue('enable_metadata', 1);
    },
    [form],
  );

  return (
    <>
      <RAGFlowFormItem
        label={t('knowledgeConfiguration.autoMetadata')}
        name="enable_metadata"
      >
        {(field) => (
          <div className="flex items-center justify-between">
            <Button
              type="button"
              variant="ghost"
              onClick={handleOpen}
              data-testid="extractor-metadata-open-modal-btn"
            >
              <div className="flex items-center gap-2">
                <Settings />
                {t('knowledgeConfiguration.settings')}
              </div>
            </Button>
            <Switch
              checked={field.value === 1 || field.value === true}
              onCheckedChange={(checked) => field.onChange(checked ? 1 : 0)}
              data-testid="extractor-metadata-switch"
            />
          </div>
        )}
      </RAGFlowFormItem>
      {manageMetadataVisible && (
        <ManageMetadataModal
          title={t('knowledgeDetails.metadata.metadataGenerationSettings')}
          visible={manageMetadataVisible}
          hideModal={hideManageMetadataModal}
          tableData={tableData}
          isCanAdd={metadataConfig.isCanAdd}
          isDeleteSingleValue={metadataConfig.isDeleteSingleValue}
          type={metadataConfig.type}
          otherData={metadataConfig.record}
          isShowDescription
          isShowValueSwitch
          isVerticalShowValue={false}
          builtInMetadata={metadataConfig.builtInMetadata}
          secondTitle={metadataConfig.secondTitle}
          success={handleSave}
        />
      )}
    </>
  );
}

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

  const { treeData, loadData } = useTagFileTree(form.watch('tag_file_id'));

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

        {(node?.data as Record<string, any>)?.operatorId ===
          BuiltinAutoExtractNodeId && <ExtractorAutoMetadata />}

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

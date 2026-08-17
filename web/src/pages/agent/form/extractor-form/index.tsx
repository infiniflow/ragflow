/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '@/components/auto-keywords-form-field';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { AsyncTreeSelect } from '@/components/ui/async-tree-select';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { FormLayout } from '@/constants/form';
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
import { zodResolver } from '@hookform/resolvers/zod';
import { Settings } from 'lucide-react';
import { memo, useCallback, useEffect, useState } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialExtractorValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useBuildNodeOutputOptions } from '../../hooks/use-build-options';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { canSelectTagFile, useTagFileTree } from './use-tag-file-tree';

export const FormSchema = z.object({
  field_name: z.string().optional(),
  sys_prompt: z.string().optional(),
  prompts: z.string().optional(),
  auto_keywords: z.number().optional(),
  auto_questions: z.number().optional(),
  auto_tags: z.number().optional(),
  tag_file_id: z.string().optional(),
  enable_summary: z.union([z.number(), z.boolean()]).optional(),
  // Builtin auto-metadata (mirrors Python's Auto metadata): enable_metadata
  // toggle + metadata / built_in_metadata field schema, consumed by the Go
  // extractor's runEnableMetadata at parse time.
  enable_metadata: z.number().optional(),
  metadata: z.any().optional(),
  built_in_metadata: z.any().optional(),
  ...LlmSettingSchema,
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

enum ExtractorSubTab {
  Keywords = 'keywords',
  Questions = 'questions',
  Tags = 'tags',
  Summary = 'summary',
  Metadata = 'metadata',
}

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
  });

  const [activeTab, setActiveTab] = useState<ExtractorSubTab>(
    (form.getValues('field_name') as ExtractorSubTab) || ExtractorSubTab.Keywords,
  );

  const promptOptions = useBuildNodeOutputOptions(node?.id);

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const ownerTenantId = useOwnerTenantId();

  const { treeData, loadData } = useTagFileTree(form.watch('tag_file_id'));

  useEffect(() => {
    if (
      !form.getValues('sys_prompt') &&
      (activeTab === ExtractorSubTab.Keywords ||
        activeTab === ExtractorSubTab.Questions ||
        activeTab === ExtractorSubTab.Summary)
    ) {
      form.setValue('sys_prompt', t(`flow.prompts.system.${activeTab}`));
    }
  }, [activeTab, form, t]);

  const handleTabChange = useCallback(
    (tab: string) => {
      const newTab = tab as ExtractorSubTab;
      const prevDefaultSys = t(`flow.prompts.system.${activeTab}`);
      const currentSys = form.getValues('sys_prompt');

      setActiveTab(newTab);
      form.setValue('field_name', tab, { shouldDirty: true });

      if (
        newTab === ExtractorSubTab.Keywords ||
        newTab === ExtractorSubTab.Questions ||
        newTab === ExtractorSubTab.Summary
      ) {
        if (!currentSys || currentSys === prevDefaultSys) {
          form.setValue('sys_prompt', t(`flow.prompts.system.${newTab}`), {
            shouldDirty: true,
          });
        }
      }
    },
    [activeTab, form, t],
  );

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>

        <Tabs value={activeTab} onValueChange={handleTabChange} className="w-full">
          <TabsList className="w-full justify-start">
            <TabsTrigger value={ExtractorSubTab.Keywords}>
              {t('flow.keywords')}
            </TabsTrigger>
            <TabsTrigger value={ExtractorSubTab.Questions}>
              {t('flow.questions')}
            </TabsTrigger>
            <TabsTrigger value={ExtractorSubTab.Tags}>
              {t('flow.tags') || t('knowledgeDetails.autoTags')}
            </TabsTrigger>
            <TabsTrigger value={ExtractorSubTab.Summary}>
              {t('flow.summary')}
            </TabsTrigger>
            <TabsTrigger value={ExtractorSubTab.Metadata}>
              {t('flow.metadata')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value={ExtractorSubTab.Keywords} className="space-y-4 pt-2">
            <AutoKeywordsFormField name="auto_keywords" />
            <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={true}
                baseOptions={promptOptions}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent value={ExtractorSubTab.Questions} className="space-y-4 pt-2">
            <AutoQuestionsFormField name="auto_questions" />
            <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={true}
                baseOptions={promptOptions}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent value={ExtractorSubTab.Tags} className="space-y-4 pt-2">
            <SliderInputFormField
              name="auto_tags"
              label={t('knowledgeDetails.autoTags')}
              min={0}
              max={10}
              defaultValue={0}
              layout={FormLayout.Vertical}
            />
            <RAGFlowFormItem label={t('flow.tagFile')} name="tag_file_id">
              {(field) => (
                <AsyncTreeSelect
                  treeData={treeData}
                  value={field.value}
                  onChange={field.onChange}
                  loadData={loadData}
                  canSelect={canSelectTagFile}
                />
              )}
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent value={ExtractorSubTab.Summary} className="space-y-4 pt-2">
            <RAGFlowFormItem
              label={t('flow.enableSummary')}
              name="enable_summary"
              horizontal
              valueClassName="w-auto flex justify-end"
            >
              {(field) => (
                <Switch
                  checked={field.value === 1 || field.value === true}
                  onCheckedChange={(checked) => {
                    field.onChange(checked ? 1 : 0);
                    form.setValue('field_name', checked ? 'summary' : '', {
                      shouldDirty: true,
                    });
                  }}
                  data-testid="extractor-summary-switch"
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem label={t('flow.systemPrompt')} name="sys_prompt">
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={true}
                baseOptions={promptOptions}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent value={ExtractorSubTab.Metadata} className="space-y-4 pt-2">
            <ExtractorAutoMetadata />
          </TabsContent>
        </Tabs>

        {!hideOutputs && <Output list={outputList}></Output>}
      </FormWrapper>
    </Form>
  );
};

export default memo(ExtractorForm);

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
import { RAGFlowNodeType } from '@/interfaces/database/agent';
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
import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialGoExtractorValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { transformExtractorConfigToForm } from '@/utils/pipeline-operator';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { canSelectTagFile, useTagFileTree } from './use-tag-file-tree';

export const FormSchema = z.object({
  keywords: z
    .object({
      top_n: z.number().optional(),
      system_prompt: z.string().optional(),
    })
    .optional(),
  questions: z
    .object({
      top_n: z.number().optional(),
      system_prompt: z.string().optional(),
    })
    .optional(),
  tags: z
    .object({
      top_n: z.number().optional(),
      tag_file_id: z.string().optional(),
    })
    .optional(),
  summary: z
    .object({
      enabled: z.union([z.number(), z.boolean()]).optional(),
      system_prompt: z.string().optional(),
    })
    .optional(),
  metadata_config: z
    .object({
      enabled: z.union([z.number(), z.boolean()]).optional(),
      metadata: z.any().optional(),
      built_in_metadata: z.any().optional(),
    })
    .optional(),

  // Legacy flat fields for backward compatibility
  field_name: z.string().optional(),
  sys_prompt: z.string().optional(),
  prompts: z.string().optional(),
  keywords_sys_prompt: z.string().optional(),
  questions_sys_prompt: z.string().optional(),
  auto_keywords: z.number().optional(),
  auto_questions: z.number().optional(),
  auto_tags: z.number().optional(),
  tag_file_id: z.string().optional(),
  enable_summary: z.union([z.number(), z.boolean()]).optional(),
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
        form.getValues('metadata_config.metadata') ||
          form.getValues('metadata') ||
          [],
      ),
      isCanAdd: true,
      type: MetadataType.Setting,
      builtInMetadata:
        form.getValues('metadata_config.built_in_metadata') ||
        form.getValues('built_in_metadata') ||
        [],
    });
  }, [form, showManageMetadataModal]);

  const handleSave = useCallback(
    (data?: {
      metadata?: IMetaDataReturnJSONSettings;
      builtInMetadata?: IBuiltInMetadataItem[];
    }) => {
      const metaList = data?.metadata || [];
      const builtInList = data?.builtInMetadata || [];
      form.setValue('metadata_config.metadata', metaList, {
        shouldDirty: true,
      });
      form.setValue('metadata_config.built_in_metadata', builtInList, {
        shouldDirty: true,
      });
      form.setValue('metadata_config.enabled', true, { shouldDirty: true });
      // Also keep flat fields for backward compatibility
      form.setValue('metadata', metaList, { shouldDirty: true });
      form.setValue('built_in_metadata', builtInList, { shouldDirty: true });
      form.setValue('enable_metadata', 1, { shouldDirty: true });
    },
    [form],
  );

  return (
    <>
      <RAGFlowFormItem
        label={t('knowledgeConfiguration.autoMetadata')}
        name="metadata_config.enabled"
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
              onCheckedChange={(checked) => {
                field.onChange(checked);
                form.setValue('enable_metadata', checked ? 1 : 0, {
                  shouldDirty: true,
                });
              }}
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

const outputList = buildOutputList(initialGoExtractorValues.outputs);

const useNormalizedExtractorFormValues = (node?: RAGFlowNodeType) => {
  return useMemo(() => {
    const raw = (node?.data?.form as Record<string, any>) || {};
    return {
      ...initialGoExtractorValues,
      ...transformExtractorConfigToForm(raw),
    };
  }, [node?.data?.form]);
};

const GoExtractorForm = ({
  node,
  onValuesChange,
  hideOutputs,
}: INextOperatorForm) => {
  const defaultValues = useNormalizedExtractorFormValues(node);
  const { t } = useTranslation();

  const form = useForm<ExtractorFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useEffect(() => {
    form.reset(defaultValues);
  }, [defaultValues, form]);

  const [activeTab, setActiveTab] = useState<ExtractorSubTab>(
    ExtractorSubTab.Keywords,
  );

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const ownerTenantId = useOwnerTenantId();

  const tagFileIdWatch =
    form.watch('tags.tag_file_id') || form.watch('tag_file_id');
  const { treeData, loadData } = useTagFileTree(tagFileIdWatch);

  useEffect(() => {
    if (!form.getValues('keywords.system_prompt')) {
      form.setValue(
        'keywords.system_prompt',
        t('flow.prompts.system.keywords'),
      );
    }
    if (!form.getValues('questions.system_prompt')) {
      form.setValue(
        'questions.system_prompt',
        t('flow.prompts.system.questions'),
      );
    }
    if (!form.getValues('summary.system_prompt')) {
      form.setValue('summary.system_prompt', t('flow.prompts.system.summary'));
    }
  }, [form, t]);

  const handleTabChange = useCallback((tab: string) => {
    setActiveTab(tab as ExtractorSubTab);
  }, []);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>

        <Tabs
          value={activeTab}
          onValueChange={handleTabChange}
          className="w-full"
        >
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

          <TabsContent
            value={ExtractorSubTab.Keywords}
            className="space-y-4 pt-2"
          >
            <AutoKeywordsFormField name="keywords.top_n" />
            <RAGFlowFormItem
              label={t('flow.systemPrompt')}
              name="keywords.system_prompt"
            >
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={false}
                showMergePath={false}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent
            value={ExtractorSubTab.Questions}
            className="space-y-4 pt-2"
          >
            <AutoQuestionsFormField name="questions.top_n" />
            <RAGFlowFormItem
              label={t('flow.systemPrompt')}
              name="questions.system_prompt"
            >
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={false}
                showMergePath={false}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent value={ExtractorSubTab.Tags} className="space-y-4 pt-2">
            <SliderInputFormField
              name="tags.top_n"
              label={t('knowledgeDetails.autoTags')}
              min={0}
              max={10}
              defaultValue={0}
              layout={FormLayout.Vertical}
            />
            <RAGFlowFormItem label={t('flow.tagFile')} name="tags.tag_file_id">
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

          <TabsContent
            value={ExtractorSubTab.Summary}
            className="space-y-4 pt-2"
          >
            <RAGFlowFormItem
              label={t('flow.enableSummary')}
              name="summary.enabled"
              horizontal
              valueClassName="w-auto flex justify-end"
            >
              {(field) => (
                <Switch
                  checked={field.value === 1 || field.value === true}
                  onCheckedChange={(checked) => {
                    field.onChange(checked);
                    form.setValue('field_name', checked ? 'summary' : '', {
                      shouldDirty: true,
                    });
                    form.setValue('enable_summary', checked ? 1 : 0, {
                      shouldDirty: true,
                    });
                  }}
                  data-testid="extractor-summary-switch"
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              label={t('flow.systemPrompt')}
              name="summary.system_prompt"
            >
              <PromptEditor
                placeholder={t('flow.messagePlaceholder')}
                showToolbar={false}
                showMergePath={false}
              />
            </RAGFlowFormItem>
          </TabsContent>

          <TabsContent
            value={ExtractorSubTab.Metadata}
            className="space-y-4 pt-2"
          >
            <ExtractorAutoMetadata />
          </TabsContent>
        </Tabs>

        {!hideOutputs && <Output list={outputList}></Output>}
      </FormWrapper>
    </Form>
  );
};

export default memo(GoExtractorForm);

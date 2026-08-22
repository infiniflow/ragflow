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
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { AsyncTreeSelect } from '@/components/ui/async-tree-select';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
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
  // Matches schema.ExtractorParam.Metadata on the Go side
  // (internal/ingestion/component/schema/extractor.go).
  metadata: z
    .object({
      enabled: z.union([z.number(), z.boolean()]).optional(),
      metadata: z.any().optional(),
      built_in_metadata: z.any().optional(),
    })
    .optional(),
  ...LlmSettingSchema,
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

enum ExtractorSection {
  Keywords = 'keywords',
  Questions = 'questions',
  Tags = 'tags',
  Summary = 'summary',
  Metadata = 'metadata',
}

// ExtractorAutoMetadata mirrors Python's dataset "Auto metadata" control: an
// enable switch plus a field-schema editor (custom + built-in). Values are
// stored as the nested `metadata` group ({enabled, metadata,
// built_in_metadata}) that the Go extractor's runEnableMetadata reads.
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
        form.getValues('metadata.metadata') || [],
      ),
      isCanAdd: true,
      type: MetadataType.Setting,
      builtInMetadata: form.getValues('metadata.built_in_metadata') || [],
    });
  }, [form, showManageMetadataModal]);

  const handleSave = useCallback(
    (data?: {
      metadata?: IMetaDataReturnJSONSettings;
      builtInMetadata?: IBuiltInMetadataItem[];
    }) => {
      form.setValue('metadata.metadata', data?.metadata || [], {
        shouldDirty: true,
      });
      form.setValue('metadata.built_in_metadata', data?.builtInMetadata || [], {
        shouldDirty: true,
      });
      form.setValue('metadata.enabled', true, { shouldDirty: true });
    },
    [form],
  );

  return (
    <>
      <RAGFlowFormItem
        label={t('knowledgeConfiguration.autoMetadata')}
        name="metadata.enabled"
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
              onCheckedChange={field.onChange}
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

  const [activeSection, setActiveSection] = useState<ExtractorSection>(
    ExtractorSection.Keywords,
  );

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const ownerTenantId = useOwnerTenantId();

  const tagFileIdWatch = form.watch('tags.tag_file_id');
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

  const sectionOptions = useMemo(
    () => [
      { label: t('flow.keywords'), value: ExtractorSection.Keywords },
      { label: t('flow.questions'), value: ExtractorSection.Questions },
      {
        label: t('flow.tags') || t('knowledgeDetails.autoTags'),
        value: ExtractorSection.Tags,
      },
      { label: t('flow.summary'), value: ExtractorSection.Summary },
      { label: t('flow.metadata'), value: ExtractorSection.Metadata },
    ],
    [t],
  );

  const handleSectionChange = useCallback((section: string) => {
    setActiveSection(section as ExtractorSection);
  }, []);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>

        <div>
          <SelectWithSearch
            value={activeSection}
            onChange={handleSectionChange}
            options={sectionOptions}
          />

          <div className="space-y-4 pt-4">
            {activeSection === ExtractorSection.Keywords && (
              <>
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
              </>
            )}

            {activeSection === ExtractorSection.Questions && (
              <>
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
              </>
            )}

            {activeSection === ExtractorSection.Tags && (
              <>
                <SliderInputFormField
                  name="tags.top_n"
                  label={t('knowledgeDetails.autoTags')}
                  min={0}
                  max={10}
                  defaultValue={0}
                  layout={FormLayout.Vertical}
                />
                <RAGFlowFormItem
                  label={t('flow.tagFile')}
                  name="tags.tag_file_id"
                >
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
              </>
            )}

            {activeSection === ExtractorSection.Summary && (
              <>
                <RAGFlowFormItem
                  label={t('flow.enableSummary')}
                  name="summary.enabled"
                  horizontal
                  valueClassName="w-auto flex justify-end"
                >
                  {(field) => (
                    <Switch
                      checked={field.value === 1 || field.value === true}
                      onCheckedChange={field.onChange}
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
              </>
            )}

            {activeSection === ExtractorSection.Metadata && (
              <ExtractorAutoMetadata />
            )}
          </div>
        </div>

        {!hideOutputs && <Output list={outputList}></Output>}
      </FormWrapper>
    </Form>
  );
};

export default memo(GoExtractorForm);

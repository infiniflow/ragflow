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
import { Collapse } from '@/components/collapse';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { useSyncExternalFormErrors } from '@/components/pipeline-operator-tabs/use-sync-external-form-errors';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import { AsyncTreeSelect } from '@/components/ui/async-tree-select';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { FormLayout } from '@/constants/form';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
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
import { transformExtractorConfigToForm } from '@/utils/pipeline-operator';
import { zodResolver } from '@hookform/resolvers/zod';
import { Settings } from 'lucide-react';
import { memo, useCallback, useEffect, useMemo } from 'react';
import { useForm, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialGoExtractorValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
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
  // No required mark on the model field — an LLM is optional for extraction.
  llm_id: z.string().optional(),
});

export type ExtractorFormSchemaType = z.infer<typeof FormSchema>;

// The summary/metadata enable switches live in the Collapse header
// (rightContent); the metadata section body only carries the field-schema
// editor (custom + built-in metadata).
function ExtractorMetadataContent() {
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
      <div>
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
      </div>
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
          isLocalSave
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
  externalErrors,
}: INextOperatorForm) => {
  const defaultValues = useNormalizedExtractorFormValues(node);
  const { t } = useTranslation();

  const form = useForm<ExtractorFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useSyncExternalFormErrors(form, externalErrors);

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const ownerTenantId = useOwnerTenantId();

  const tagFileId = useWatch({
    control: form.control,
    name: 'tags.tag_file_id',
  });
  const { treeData, loadData } = useTagFileTree(tagFileId);

  const summaryEnabledRaw = useWatch({
    control: form.control,
    name: 'summary.enabled',
  });
  const metadataEnabledRaw = useWatch({
    control: form.control,
    name: 'metadata.enabled',
  });
  const summaryEnabled = summaryEnabledRaw === true || summaryEnabledRaw === 1;
  const metadataEnabled =
    metadataEnabledRaw === true || metadataEnabledRaw === 1;

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

  const handleSummarySwitch = useCallback(
    (checked: boolean) => {
      form.setValue('summary.enabled', checked, { shouldDirty: true });
    },
    [form],
  );

  const handleMetadataSwitch = useCallback(
    (checked: boolean) => {
      form.setValue('metadata.enabled', checked, { shouldDirty: true });
    },
    [form],
  );

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>

        <div className="space-y-4">
          <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
            <Collapse title={t('flow.keywords')} defaultOpen>
              <div className="space-y-4">
                <AutoKeywordsFormField name="keywords.top_n" />
                <RAGFlowFormItem
                  label={t('flow.systemPrompt')}
                  name="keywords.system_prompt"
                >
                  <Textarea
                    placeholder={t('flow.messagePlaceholder')}
                    rows={18}
                    resize="vertical"
                  />
                </RAGFlowFormItem>
              </div>
            </Collapse>
          </Card>

          <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
            <Collapse title={t('flow.questions')} defaultOpen>
              <div className="space-y-4">
                <AutoQuestionsFormField name="questions.top_n" />
                <RAGFlowFormItem
                  label={t('flow.systemPrompt')}
                  name="questions.system_prompt"
                >
                  <Textarea
                    placeholder={t('flow.messagePlaceholder')}
                    resize="vertical"
                    rows={18}
                  />
                </RAGFlowFormItem>
              </div>
            </Collapse>
          </Card>

          <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
            <Collapse
              title={t('flow.tags') || t('knowledgeDetails.autoTags')}
              defaultOpen
            >
              <div className="space-y-4">
                <SliderInputFormField
                  name="tags.top_n"
                  label={t('knowledgeDetails.autoTags')}
                  min={0}
                  max={10}
                  defaultValue={0}
                  layout={FormLayout.Vertical}
                  integer
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
              </div>
            </Collapse>
          </Card>

          <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
            <Collapse
              title={t('flow.summary')}
              defaultOpen
              rightContent={
                <Switch
                  checked={summaryEnabled}
                  onCheckedChange={handleSummarySwitch}
                  data-testid="extractor-summary-switch"
                />
              }
            >
              <RAGFlowFormItem
                label={t('flow.systemPrompt')}
                name="summary.system_prompt"
              >
                <Textarea
                  placeholder={t('flow.messagePlaceholder')}
                  resize="vertical"
                  rows={18}
                />
              </RAGFlowFormItem>
            </Collapse>
          </Card>

          <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
            <Collapse
              title={t('flow.metadata')}
              defaultOpen
              rightContent={
                <Switch
                  checked={metadataEnabled}
                  onCheckedChange={handleMetadataSwitch}
                  data-testid="extractor-metadata-switch"
                />
              }
            >
              <ExtractorMetadataContent />
            </Collapse>
          </Card>
        </div>

        {!hideOutputs && <Output list={outputList}></Output>}
      </FormWrapper>
    </Form>
  );
};

export default memo(GoExtractorForm);

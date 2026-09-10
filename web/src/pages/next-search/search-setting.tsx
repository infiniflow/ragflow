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

// src/pages/next-search/search-setting.tsx

import AvatarNameDescription from '@/components/avatar-name-description';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { LlmSettingFieldItems } from '@/components/llm-setting-items/next';
import { MetadataFilter } from '@/components/metadata-filter';
import { RerankCandidatesCountFormField } from '@/components/rerank-candidates-count-item';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { Spin } from '@/components/ui/spin';
import { Switch } from '@/components/ui/switch';
import { useFetchKnowledgeMetadataKeys } from '@/hooks/use-knowledge-request';
import {
  useRevalidateStaleDatasetIds,
  useStaleDatasetFormSchema,
} from '@/hooks/use-stale-dataset-validation';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  ISearchAppDetailProps,
  IUpdateSearchProps,
  IllmSettingProps,
  useUpdateSearch,
} from '../next-searches/hooks';
import { RerankFormFields } from '@/components/rerank';
import { resolveInitialLlmSetting } from './llm-setting-defaults';
import {
  SearchSettingFormData,
  useRevalidatePersistedModels,
  useSearchSettingFormSchema,
} from './search-setting-hooks';

interface SearchSettingProps {
  open: boolean;
  setOpen: (open: boolean) => void;
  className?: string;
  data: ISearchAppDetailProps;
}

function SearchSetting({
  open = false,
  setOpen,
  className,
  data,
}: SearchSettingProps) {
  const [width0, setWidth0] = useState('w-[440px]');
  const { search_config } = data || {};
  const { llm_setting } = search_config || {};
  const { t } = useTranslation();
  const { formSchema: searchSettingSchema, modelsFetched } =
    useSearchSettingFormSchema();
  const { formSchema, datasetsFetched } = useStaleDatasetFormSchema(
    searchSettingSchema,
    search_config?.kb_ids,
    'search_config.kb_ids',
  );
  const formMethods = useForm<SearchSettingFormData>({
    resolver: zodResolver(formSchema),
    mode: 'onChange',
  });

  useRevalidateStaleDatasetIds(
    formMethods,
    datasetsFetched,
    'search_config.kb_ids',
  );
  const descriptionDefaultValue = t('search.descriptionValue');
  const resetForm = useCallback(() => {
    formMethods.reset({
      search_id: data?.id,
      name: data?.name || '',
      avatar: data?.avatar || '',
      description: data?.description || descriptionDefaultValue,
      search_config: {
        kb_ids: search_config?.kb_ids || [],
        vector_similarity_weight:
          search_config?.vector_similarity_weight ?? 0.3,
        web_search: search_config?.web_search || false,
        doc_ids: [],
        similarity_threshold: search_config?.similarity_threshold ?? 0.2,
        use_kg: false,
        rerank_id: search_config?.rerank_id || '',
        use_rerank: search_config?.rerank_id ? true : false,
        rerank_candidates_count: search_config?.rerank_candidates_count ?? 100,
        summary: search_config?.summary || false,
        chat_id: search_config?.chat_id || '',
        llm_setting: {
          llm_id: search_config?.chat_id || '',
          parameter: llm_setting?.parameter || '',
          ...resolveInitialLlmSetting(llm_setting),
        },
        chat_settingcross_languages: [],
        highlight: false,
        keyword: false,
        related_search: search_config?.related_search || false,
        query_mindmap: search_config?.query_mindmap || false,
        meta_data_filter: search_config?.meta_data_filter,
        reference_metadata: {
          include: search_config?.reference_metadata?.include || false,
          fields:
            search_config?.reference_metadata?.fields &&
            search_config.reference_metadata.fields.length > 0
              ? search_config.reference_metadata.fields
              : undefined,
        },
      },
      temperatureEnabled: llm_setting?.temperature_enabled ?? true,
      topPEnabled: llm_setting?.top_p_enabled ?? true,
      presencePenaltyEnabled: llm_setting?.presence_penalty_enabled ?? true,
      frequencyPenaltyEnabled: llm_setting?.frequency_penalty_enabled ?? true,
    });
  }, [data, search_config, llm_setting, formMethods, descriptionDefaultValue]);

  useEffect(() => {
    resetForm();
  }, [resetForm]);

  useEffect(() => {
    if (!open) {
      setTimeout(() => {
        setWidth0('w-0 hidden');
      }, 500);
    } else {
      setWidth0('w-[440px]');
    }
  }, [open]);

  const { rerankModelEnabled, aiSummaryEnabled } = useRevalidatePersistedModels(
    {
      control: formMethods.control,
      trigger: formMethods.trigger,
      modelsFetched,
    },
  );

  const selectedKbIds = useWatch({
    control: formMethods.control,
    name: 'search_config.kb_ids',
  });
  const referenceMetadataEnabled = useWatch({
    control: formMethods.control,
    name: 'search_config.reference_metadata.include',
  });
  const { data: metadataKeys, loading: metadataKeysLoading } =
    useFetchKnowledgeMetadataKeys(selectedKbIds || []);
  const metadataFieldOptions = useMemo(() => {
    return (metadataKeys || []).map((key) => ({
      label: key,
      value: key,
    }));
  }, [metadataKeys]);

  useEffect(() => {
    const currentFields = formMethods.getValues(
      'search_config.reference_metadata.fields',
    );
    if (
      referenceMetadataEnabled &&
      Array.isArray(currentFields) &&
      currentFields.length > 0 &&
      metadataKeys
    ) {
      const validFields = currentFields.filter((field) =>
        metadataKeys.includes(field),
      );
      if (validFields.length !== currentFields.length) {
        formMethods.setValue(
          'search_config.reference_metadata.fields',
          validFields,
        );
      }
    } else if (!referenceMetadataEnabled) {
      formMethods.setValue(
        'search_config.reference_metadata.fields',
        undefined,
      );
    }
  }, [
    selectedKbIds,
    metadataKeys,
    metadataKeysLoading,
    referenceMetadataEnabled,
    formMethods,
  ]);

  const { updateSearch } = useUpdateSearch();
  const [formSubmitLoading, setFormSubmitLoading] = useState(false);
  const { data: systemSetting } = useFetchTenantInfo();
  const onSubmit = async (
    formData: IUpdateSearchProps & { tenant_id: string },
  ) => {
    try {
      setFormSubmitLoading(true);
      const {
        search_config,
        temperatureEnabled: _temperatureEnabled,
        topPEnabled: _topPEnabled,
        presencePenaltyEnabled: _presencePenaltyEnabled,
        frequencyPenaltyEnabled: _frequencyPenaltyEnabled,
        maxTokensEnabled: _maxTokensEnabled,
        ...other_formdata
      } = formData as IUpdateSearchProps & {
        tenant_id: string;
        temperatureEnabled?: boolean;
        topPEnabled?: boolean;
        presencePenaltyEnabled?: boolean;
        frequencyPenaltyEnabled?: boolean;
        maxTokensEnabled?: boolean;
      };
      void _maxTokensEnabled;
      const {
        llm_setting,
        vector_similarity_weight,
        use_rerank,
        rerank_id,
        ...other_config
      } = search_config;
      const llmSetting = {
        // llm_id: llm_setting.llm_id,
        parameter: llm_setting.parameter,
        temperature: llm_setting.temperature,
        top_p: llm_setting.top_p,
        frequency_penalty: llm_setting.frequency_penalty,
        presence_penalty: llm_setting.presence_penalty,
        temperature_enabled: _temperatureEnabled,
        top_p_enabled: _topPEnabled,
        frequency_penalty_enabled: _frequencyPenaltyEnabled,
        presence_penalty_enabled: _presencePenaltyEnabled,
      } as IllmSettingProps;
      const referenceMetadata = other_config.reference_metadata;
      const normalizedReferenceMetadata = referenceMetadata
        ? {
            ...referenceMetadata,
            ...(Array.isArray(referenceMetadata.fields) &&
            referenceMetadata.fields.length === 0
              ? { fields: undefined }
              : {}),
          }
        : referenceMetadata;

      await updateSearch({
        ...other_formdata,
        search_config: {
          ...other_config,
          reference_metadata: normalizedReferenceMetadata,
          chat_id: llm_setting.llm_id,
          vector_similarity_weight,
          rerank_id: use_rerank ? rerank_id : '',
          llm_setting: { ...llmSetting },
        },
        tenant_id: systemSetting.tenant_id,
      });
      setOpen(false);
    } catch (error) {
      console.error('Failed to update search:', error);
    } finally {
      setFormSubmitLoading(false);
    }
  };
  return (
    <div
      className={cn(
        'text-text-primary border-l-0.5 p-4 pb-12',
        {
          'animate-fade-in-right': open,
          'animate-fade-out-right': !open,
        },
        width0,
        className,
      )}
    >
      <div className="flex justify-between items-center text-base mb-8">
        <div className="text-text-primary">{t('search.searchSettings')}</div>
        <div onClick={() => setOpen(false)}>
          <X size={16} className="text-text-primary cursor-pointer" />
        </div>
      </div>
      <div
        style={{ maxHeight: 'calc(100dvh - 270px)' }}
        className="overflow-y-auto scrollbar-auto p-1 text-text-secondary"
      >
        <Form {...formMethods}>
          <form
            onSubmit={formMethods.handleSubmit(
              (data) => {
                onSubmit(data as unknown as IUpdateSearchProps);
              },
              (error) => {
                console.error(error, formMethods.getValues());
              },
            )}
            className="space-y-6"
          >
            <AvatarNameDescription avatarField="avatar" />

            <KnowledgeBaseFormField
              name="search_config.kb_ids"
              required
            ></KnowledgeBaseFormField>
            <MetadataFilter prefix="search_config."></MetadataFilter>
            <FormField
              control={formMethods.control}
              name="search_config.reference_metadata.include"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={(value) => {
                        field.onChange(value);
                        if (!value) {
                          formMethods.setValue(
                            'search_config.reference_metadata.fields',
                            undefined,
                          );
                        }
                      }}
                    />
                  </FormControl>
                  <FormLabel tooltip={t('chat.showChunkMetadataTip')}>
                    {t('chat.showChunkMetadata')}
                  </FormLabel>
                </FormItem>
              )}
            />
            {referenceMetadataEnabled && (
              <FormField
                control={formMethods.control}
                name="search_config.reference_metadata.fields"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel tooltip={t('chat.metadataFieldsTip')}>
                      {t('chat.metadataFields')}
                    </FormLabel>
                    <FormControl className="bg-bg-input">
                      <MultiSelect
                        options={metadataFieldOptions}
                        onValueChange={field.onChange}
                        showSelectAll={false}
                        placeholder={t('common.pleaseSelect')}
                        maxCount={20}
                        defaultValue={
                          Array.isArray(field.value) ? field.value : []
                        }
                        value={Array.isArray(field.value) ? field.value : []}
                        name={field.name}
                        ref={field.ref}
                        onBlur={field.onBlur}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <SimilaritySliderFormField
              isTooltipShown
              similarityName="search_config.similarity_threshold"
              similarityWeightName="search_config.vector_similarity_weight"
              numberInputClassName="rounded-sm"
            ></SimilaritySliderFormField>
            <RerankCandidatesCountFormField
              name="search_config.rerank_candidates_count"
              defaultValue={100}
            ></RerankCandidatesCountFormField>
            {/* Rerank Model */}
            <FormField
              control={formMethods.control}
              name="search_config.use_rerank"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>{t('search.rerankModel')}</FormLabel>
                </FormItem>
              )}
            />
            {rerankModelEnabled && (
              <>
                <RerankFormFields
                  prefix={'search_config.'}
                  required
                ></RerankFormFields>
              </>
            )}
            {/* AI Summary */}
            <FormField
              control={formMethods.control}
              name="search_config.summary"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>{t('search.AISummary')}</FormLabel>
                </FormItem>
              )}
            />
            {aiSummaryEnabled && (
              <LlmSettingFieldItems
                prefix="search_config.llm_setting"
                llmRequired
                showFields={[
                  'temperature',
                  'top_p',
                  'presence_penalty',
                  'frequency_penalty',
                ]}
              ></LlmSettingFieldItems>
            )}
            {/* Feature Controls */}
            {/* <FormField
              control={formMethods.control}
              name="search_config.web_search"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>{t('search.enableWebSearch')}</FormLabel>
                </FormItem>
              )}
            /> */}

            <FormField
              control={formMethods.control}
              name="search_config.related_search"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>{t('search.enableRelatedSearch')}</FormLabel>
                </FormItem>
              )}
            />
            <FormField
              control={formMethods.control}
              name="search_config.query_mindmap"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel>{t('search.showQueryMindmap')}</FormLabel>
                </FormItem>
              )}
            />
            {/* Submit Button */}
            <div className="flex justify-end"></div>
            <div className="flex justify-end gap-2 absolute bottom-1 right-3 bg-bg-base w-[calc(100%-1em)] py-2">
              <Button
                type="reset"
                variant={'transparent'}
                onClick={() => {
                  resetForm();
                  setOpen(false);
                }}
              >
                {t('search.cancelText')}
              </Button>
              <Button
                data-testid="search-settings-save"
                type="submit"
                disabled={formSubmitLoading}
              >
                {formSubmitLoading && (
                  <div className="size-4">
                    <Spin size="small" />
                  </div>
                )}
                {t('search.okText')}
              </Button>
            </div>
          </form>
        </Form>
      </div>
    </div>
  );
}

export { SearchSetting };

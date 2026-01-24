// src/pages/next-search/search-setting.tsx

import { AvatarUpload } from '@/components/avatar-upload';
import {
  LlmSettingFieldItems,
  LlmSettingSchema,
} from '@/components/llm-setting-items/next';
import {
  MetadataFilter,
  MetadataFilterSchema,
} from '@/components/metadata-filter';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { Button } from '@/components/ui/button';
import { SingleFormSlider } from '@/components/ui/dual-range-slider';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import {
  MultiSelect,
  MultiSelectOptionType,
} from '@/components/ui/multi-select';
import { RAGFlowSelect } from '@/components/ui/select';
import { Spin } from '@/components/ui/spin';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import {
  useComposeLlmOptionsByModelTypes,
  useSelectLlmOptionsByModelType,
} from '@/hooks/use-llm-request';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LlmModelType } from '../dataset/dataset/constant';
import {
  ISearchAppDetailProps,
  IUpdateSearchProps,
  IllmSettingProps,
  useUpdateSearch,
} from '../next-searches/hooks';
// import {
//   LlmSettingFieldItems,
//   LlmSettingSchema,
// } from './search-setting-aisummery-config';

interface SearchSettingProps {
  open: boolean;
  setOpen: (open: boolean) => void;
  className?: string;
  data: ISearchAppDetailProps;
}

const SearchSettingFormSchema = z
  .object({
    search_id: z.string().optional(),
    name: z.string().min(1, 'Name is required'),
    avatar: z.string().optional(),
    description: z.string().optional(),
    search_config: z.object({
      kb_ids: z.array(z.string()).min(1, 'At least one dataset is required'),
      vector_similarity_weight: z.number().min(0).max(1),
      web_search: z.boolean(),
      similarity_threshold: z.number(),
      use_kg: z.boolean(),
      rerank_id: z.string(),
      use_rerank: z.boolean(),
      top_k: z.number(),
      summary: z.boolean(),
      llm_setting: z.object(LlmSettingSchema),
      related_search: z.boolean(),
      query_mindmap: z.boolean(),
      ...MetadataFilterSchema,
    }),
  })
  .superRefine((data, ctx) => {
    if (data.search_config.use_rerank && !data.search_config.rerank_id) {
      ctx.addIssue({
        path: ['search_config', 'rerank_id'],
        message: 'Rerank model is required when rerank is enabled',
        code: z.ZodIssueCode.custom,
      });
    }

    if (data.search_config.summary && !data.search_config.llm_setting?.llm_id) {
      ctx.addIssue({
        path: ['search_config', 'llm_setting', 'llm_id'],
        message: 'Model is required when AI Summary is enabled',
        code: z.ZodIssueCode.custom,
      });
    }
  });
type SearchSettingFormData = z.infer<typeof SearchSettingFormSchema>;
const SearchSetting: React.FC<SearchSettingProps> = ({
  open = false,
  setOpen,
  className,
  data,
}) => {
  const [width0, setWidth0] = useState('w-[440px]');
  const { search_config } = data || {};
  const { llm_setting } = search_config || {};
  const formMethods = useForm<SearchSettingFormData>({
    resolver: zodResolver(SearchSettingFormSchema),
  });

  const [datasetList, setDatasetList] = useState<MultiSelectOptionType[]>([]);
  const [datasetSelectEmbdId, setDatasetSelectEmbdId] = useState('');
  const { t } = useTranslation();
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
          (search_config?.vector_similarity_weight
            ? 1 - search_config?.vector_similarity_weight
            : 0.3) || 0.3,
        web_search: search_config?.web_search || false,
        doc_ids: [],
        similarity_threshold: search_config?.similarity_threshold || 0.2,
        use_kg: false,
        rerank_id: search_config?.rerank_id || '',
        use_rerank: search_config?.rerank_id ? true : false,
        top_k: search_config?.top_k || 1024,
        summary: search_config?.summary || false,
        chat_id: search_config?.chat_id || '',
        llm_setting: {
          llm_id: search_config?.chat_id || '',
          parameter: llm_setting?.parameter,
          temperature: llm_setting?.temperature || 0,
          top_p: llm_setting?.top_p || 0,
          frequency_penalty: llm_setting?.frequency_penalty || 0,
          presence_penalty: llm_setting?.presence_penalty || 0,
          temperatureEnabled: llm_setting?.temperature ? true : false,
          topPEnabled: llm_setting?.top_p ? true : false,
          presencePenaltyEnabled: llm_setting?.presence_penalty ? true : false,
          frequencyPenaltyEnabled: llm_setting?.frequency_penalty
            ? true
            : false,
        },
        chat_settingcross_languages: [],
        highlight: false,
        keyword: false,
        related_search: search_config?.related_search || false,
        query_mindmap: search_config?.query_mindmap || false,
        meta_data_filter: search_config?.meta_data_filter,
      },
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

  const { list: datasetListOrigin } = useFetchKnowledgeList();

  useEffect(() => {
    const datasetListMap = datasetListOrigin.map((item: IKnowledge) => {
      return {
        label: item.name,
        suffix: (
          <div className="text-xs px-4 p-1 bg-bg-card text-text-secondary rounded-lg border border-bg-card">
            {item.embd_id}
          </div>
        ),
        value: item.id,
        disabled:
          item.embd_id !== datasetSelectEmbdId && datasetSelectEmbdId !== '',
      };
    });
    setDatasetList(datasetListMap);
  }, [datasetListOrigin, datasetSelectEmbdId]);

  const handleDatasetSelectChange = (
    value: string[],
    onChange: (value: string[]) => void,
  ) => {
    console.log(value);
    if (value.length) {
      const data = datasetListOrigin?.find((item) => item.id === value[0]);
      setDatasetSelectEmbdId(data?.embd_id ?? '');
    } else {
      setDatasetSelectEmbdId('');
    }
    formMethods.setValue('search_config.kb_ids', value);
    onChange?.(value);
  };

  const allOptions = useSelectLlmOptionsByModelType();
  const rerankModelOptions = useMemo(() => {
    return allOptions[LlmModelType.Rerank];
  }, [allOptions]);

  const aiSummeryModelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  const rerankModelDisabled = useWatch({
    control: formMethods.control,
    name: 'search_config.use_rerank',
  });

  const aiSummaryDisabled = useWatch({
    control: formMethods.control,
    name: 'search_config.summary',
  });

  // Reset top_k to 1024 only when user actively disables rerank (from true to false)
  const prevRerankEnabled = useRef<boolean | undefined>(undefined);
  useEffect(() => {
    if (prevRerankEnabled.current === true && rerankModelDisabled === false) {
      formMethods.setValue('search_config.top_k', 1024);
    }
    prevRerankEnabled.current = rerankModelDisabled;
  }, [rerankModelDisabled, formMethods]);

  const { updateSearch } = useUpdateSearch();
  const [formSubmitLoading, setFormSubmitLoading] = useState(false);
  const { data: systemSetting } = useFetchTenantInfo();
  const onSubmit = async (
    formData: IUpdateSearchProps & { tenant_id: string },
  ) => {
    try {
      setFormSubmitLoading(true);
      const { search_config, ...other_formdata } = formData;
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
      } as IllmSettingProps;

      await updateSearch({
        ...other_formdata,
        search_config: {
          ...other_config,
          chat_id: llm_setting.llm_id,
          vector_similarity_weight: 1 - vector_similarity_weight,
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
        'text-text-primary border p-4 pb-12 rounded-lg',
        {
          'animate-fade-in-right': open,
          'animate-fade-out-right': !open,
        },
        width0,
        className,
      )}
      style={{ maxHeight: 'calc(100dvh - 170px)' }}
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
                console.log('Form submitted with data:', data);
                onSubmit(data as unknown as IUpdateSearchProps);
              },
              (errors) => {
                console.log('Validation errors:', errors);
              },
            )}
            className="space-y-6"
          >
            {/* Name */}
            <FormField
              control={formMethods.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>
                    {t('search.name')}
                  </FormLabel>
                  <FormControl>
                    <Input placeholder={t('search.name')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {/* Avatar */}
            <FormField
              control={formMethods.control}
              name="avatar"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('search.avatar')}</FormLabel>
                  <FormControl>
                    <AvatarUpload {...field}></AvatarUpload>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {/* Description */}
            <FormField
              control={formMethods.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('search.description')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={descriptionDefaultValue}
                      {...field}
                      onFocus={() => {
                        if (field.value === descriptionDefaultValue) {
                          field.onChange('');
                        }
                      }}
                      onBlur={() => {
                        if (field.value === '') {
                          field.onChange(descriptionDefaultValue);
                        }
                      }}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {/* Datasets */}
            <FormField
              control={formMethods.control}
              name="search_config.kb_ids"
              rules={{ required: 'Datasets is required' }}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>
                    {t('search.datasets')}
                  </FormLabel>
                  <FormControl className="bg-bg-input">
                    <MultiSelect
                      options={datasetList}
                      onValueChange={(value) => {
                        handleDatasetSelectChange(value, field.onChange);
                      }}
                      showSelectAll={false}
                      placeholder={t('chat.knowledgeBasesMessage')}
                      maxCount={10}
                      defaultValue={field.value}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <MetadataFilter prefix="search_config."></MetadataFilter>
            <SimilaritySliderFormField
              isTooltipShown
              similarityName="search_config.similarity_threshold"
              vectorSimilarityWeightName="search_config.vector_similarity_weight"
              numberInputClassName="rounded-sm"
            ></SimilaritySliderFormField>
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
            {rerankModelDisabled && (
              <>
                <FormField
                  control={formMethods.control}
                  name={'search_config.rerank_id'}
                  // rules={{ required: 'Model is required' }}
                  render={({ field }) => (
                    <FormItem className="flex flex-col">
                      <FormLabel>
                        <span className="text-destructive mr-1"> *</span>
                        {t('chat.model')}
                      </FormLabel>
                      <FormControl>
                        <RAGFlowSelect
                          {...field}
                          options={rerankModelOptions}
                          triggerClassName={'bg-bg-input'}
                          // disabled={disabled}
                          placeholder={t('chat.model')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={formMethods.control}
                  name="search_config.top_k"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Top K</FormLabel>
                      <div
                        className={cn(
                          'flex items-center gap-4 justify-between',
                          className,
                        )}
                      >
                        <FormControl>
                          <SingleFormSlider
                            {...field}
                            max={2048}
                            min={0}
                            step={1}
                          ></SingleFormSlider>
                        </FormControl>
                        <FormControl>
                          <Input
                            type={'number'}
                            className="h-7 w-20 bg-bg-card border border-border-button rounded-sm"
                            max={2048}
                            min={0}
                            step={1}
                            {...field}
                          ></Input>
                        </FormControl>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />
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
            {aiSummaryDisabled && (
              // <LlmSettingFieldItems
              //   prefix="search_config.llm_setting"
              //   options={aiSummeryModelOptions}
              // ></LlmSettingFieldItems>
              <LlmSettingFieldItems
                prefix="search_config.llm_setting"
                options={aiSummeryModelOptions}
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
              <Button type="submit" disabled={formSubmitLoading}>
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
};

export { SearchSetting };

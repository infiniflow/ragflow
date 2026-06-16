'use client';

import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { MetadataFilter } from '@/components/metadata-filter';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { SwitchFormField } from '@/components/switch-fom-field';
import { TavilyFormField } from '@/components/tavily-form-field';
import { TOCEnhanceFormField } from '@/components/toc-enhance-form-field';
import { TopNFormField } from '@/components/top-n-item';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { useFetchKnowledgeMetadataKeys } from '@/hooks/use-knowledge-request';
import { getDirAttribute } from '@/utils/text-direction';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { DynamicVariableForm } from './dynamic-variable';

export function ChatPromptEngine() {
  const { t } = useTranslation();
  const form = useFormContext();
  const systemPromptValue = form.watch('prompt_config.system');

  const emptyResponseValue = form.watch('prompt_config.empty_response');
  const rawDatasetIds = useWatch({
    control: form.control,
    name: 'dataset_ids',
  });
  const kbIds = useMemo(
    () => (rawDatasetIds || []) as string[],
    [rawDatasetIds],
  );
  const metadataInclude = useWatch({
    control: form.control,
    name: 'prompt_config.reference_metadata.include',
  });
  const { data: metadataKeys, loading: metadataKeysLoading } =
    useFetchKnowledgeMetadataKeys(kbIds);
  const metadataFieldOptions = useMemo(() => {
    return (metadataKeys || []).map((key) => ({
      label: key,
      value: key,
    }));
  }, [metadataKeys]);

  useEffect(() => {
    const currentFields = form.getValues(
      'prompt_config.reference_metadata.fields',
    );
    if (
      metadataInclude &&
      Array.isArray(currentFields) &&
      currentFields.length > 0 &&
      metadataKeys
    ) {
      const validFields = currentFields.filter((field) =>
        metadataKeys.includes(field),
      );
      if (validFields.length !== currentFields.length) {
        form.setValue('prompt_config.reference_metadata.fields', validFields);
      }
    } else if (!metadataInclude) {
      form.setValue('prompt_config.reference_metadata.fields', undefined);
    }
  }, [kbIds, metadataKeys, metadataKeysLoading, metadataInclude, form]);

  return (
    <Collapse title={t('flow.advancedSettings')}>
      <div className="space-y-8">
        <FormField
          control={form.control}
          name={'prompt_config.empty_response'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.emptyResponseTip')}>
                {t('chat.emptyResponse')}
              </FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  placeholder={t('chat.emptyResponsePlaceholder')}
                  dir={getDirAttribute(emptyResponseValue || '')}
                ></Textarea>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <SwitchFormField
          name={'prompt_config.quote'}
          label={t('chat.quote')}
          tooltip={t('chat.quoteTip')}
        ></SwitchFormField>
        <SwitchFormField
          name={'prompt_config.keyword'}
          label={t('chat.keyword')}
          tooltip={t('chat.keywordTip')}
        ></SwitchFormField>
        <SwitchFormField
          name={'prompt_config.tts'}
          label={t('chat.tts')}
          tooltip={t('chat.ttsTip')}
        ></SwitchFormField>
        <TOCEnhanceFormField name="prompt_config.toc_enhance"></TOCEnhanceFormField>
        <TavilyFormField></TavilyFormField>
        <MetadataFilter></MetadataFilter>
        <FormField
          control={form.control}
          name={'prompt_config.reference_metadata.include'}
          render={({ field }) => (
            <FormItem className="flex flex-row items-start space-x-3 space-y-0">
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={(value) => {
                    field.onChange(value);
                    if (!value) {
                      form.setValue(
                        'prompt_config.reference_metadata.fields',
                        undefined,
                      );
                    }
                  }}
                />
              </FormControl>
              <FormLabel tooltip="Display document metadata (e.g., title, page number, upload date) alongside retrieved text chunks">
                Show chunk metadata
              </FormLabel>
            </FormItem>
          )}
        />
        {metadataInclude && (
          <FormField
            control={form.control}
            name={'prompt_config.reference_metadata.fields'}
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip="Select which metadata fields to display with each chunk">
                  {t('chat.metadataKeys')}
                </FormLabel>
                <FormControl className="bg-bg-input">
                  <MultiSelect
                    options={metadataFieldOptions}
                    onValueChange={field.onChange}
                    showSelectAll={false}
                    placeholder="Please select"
                    maxCount={20}
                    defaultValue={Array.isArray(field.value) ? field.value : []}
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
        <FormField
          control={form.control}
          name="prompt_config.system"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('chat.system')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={8}
                  placeholder={t('chat.systemPlaceholder')}
                  className="overflow-y-auto"
                  dir={getDirAttribute(systemPromptValue || '')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <SimilaritySliderFormField isTooltipShown></SimilaritySliderFormField>
        <TopNFormField></TopNFormField>
        <SwitchFormField
          name={'prompt_config.refine_multiturn'}
          label={t('chat.multiTurn')}
          tooltip={t('chat.multiTurnTip')}
        ></SwitchFormField>
        <UseKnowledgeGraphFormField name="prompt_config.use_kg"></UseKnowledgeGraphFormField>
        <RerankFormFields></RerankFormFields>
        <CrossLanguageFormField></CrossLanguageFormField>
        <DynamicVariableForm></DynamicVariableForm>
      </div>
    </Collapse>
  );
}

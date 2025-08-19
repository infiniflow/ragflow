'use client';

import { FileUploader } from '@/components/file-uploader';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-fom-field';
import { TavilyFormField } from '@/components/tavily-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext, useWatch } from 'react-hook-form';
import { DatasetMetadata } from '../../constants';
import { MetadataFilterConditions } from './metadata-filter-conditions';

export default function ChatBasicSetting() {
  const { t } = useTranslate('chat');
  const form = useFormContext();
  const kbIds: string[] = useWatch({ control: form.control, name: 'kb_ids' });
  const metadata = useWatch({
    control: form.control,
    name: 'meta_data_filter.method',
  });
  const hasKnowledge = Array.isArray(kbIds) && kbIds.length > 0;

  const MetadataOptions = Object.values(DatasetMetadata).map((x) => {
    return {
      value: x,
      label: t(`meta.${x}`),
    };
  });

  return (
    <div className="space-y-8">
      <FormField
        control={form.control}
        name={'icon'}
        render={({ field }) => (
          <div className="space-y-6">
            <FormItem className="w-full">
              <FormLabel>{t('assistantAvatar')}</FormLabel>
              <FormControl>
                <FileUploader
                  value={field.value}
                  onValueChange={field.onChange}
                  maxFileCount={1}
                  maxSize={4 * 1024 * 1024}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          </div>
        )}
      />
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('assistantName')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('description')}</FormLabel>
            <FormControl>
              <Textarea {...field}></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'prompt_config.empty_response'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('emptyResponse')}</FormLabel>
            <FormControl>
              <Textarea {...field}></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={'prompt_config.prologue'}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('setAnOpener')}</FormLabel>
            <FormControl>
              <Textarea {...field}></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <SwitchFormField
        name={'prompt_config.quote'}
        label={t('quote')}
      ></SwitchFormField>
      <SwitchFormField
        name={'prompt_config.keyword'}
        label={t('keyword')}
      ></SwitchFormField>
      <SwitchFormField
        name={'prompt_config.tts'}
        label={t('tts')}
      ></SwitchFormField>
      <TavilyFormField></TavilyFormField>
      <KnowledgeBaseFormField></KnowledgeBaseFormField>
      {hasKnowledge && (
        <RAGFlowFormItem
          label={t('metadata')}
          name={'meta_data_filter.method'}
          tooltip={t('metadataTip')}
        >
          <SelectWithSearch options={MetadataOptions} />
        </RAGFlowFormItem>
      )}
      {hasKnowledge && metadata === DatasetMetadata.Manual && (
        <MetadataFilterConditions kbIds={kbIds}></MetadataFilterConditions>
      )}
    </div>
  );
}

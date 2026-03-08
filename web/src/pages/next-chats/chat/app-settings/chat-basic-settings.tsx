'use client';

import { AvatarUpload } from '@/components/avatar-upload';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { MetadataFilter } from '@/components/metadata-filter';
import { SwitchFormField } from '@/components/switch-fom-field';
import { TavilyFormField } from '@/components/tavily-form-field';
import { TOCEnhanceFormField } from '@/components/toc-enhance-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { getDirAttribute } from '@/utils/text-direction';
import { useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';

export default function ChatBasicSetting() {
  const { t } = useTranslate('chat');
  const form = useFormContext();
  const nameValue = form.watch('name');
  const descriptionValue = form.watch('description');
  const emptyResponseValue = form.watch('prompt_config.empty_response');
  const prologueValue = form.watch('prompt_config.prologue');
  const kbIds = (useWatch({ control: form.control, name: 'kb_ids' }) ||
    []) as string[];
  const metadataInclude = useWatch({
    control: form.control,
    name: 'prompt_config.reference_metadata.include',
  });
  const { data: availableMetadata } = useFetchKnowledgeMetadata(kbIds);
  const metadataFieldOptions = useMemo(() => {
    return Object.keys(availableMetadata || {}).map((key) => ({
      label: key,
      value: key,
    }));
  }, [availableMetadata]);

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
                <AvatarUpload {...field}></AvatarUpload>
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
            <FormLabel required>{t('assistantName')}</FormLabel>
            <FormControl>
              <Input {...field} dir={getDirAttribute(nameValue || '')}></Input>
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
              <Textarea
                {...field}
                placeholder={t('descriptionPlaceholder')}
                dir={getDirAttribute(descriptionValue || '')}
              ></Textarea>
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
            <FormLabel tooltip={t('emptyResponseTip')}>
              {t('emptyResponse')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                placeholder={t('emptyResponsePlaceholder')}
                dir={getDirAttribute(emptyResponseValue || '')}
              ></Textarea>
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
            <FormLabel tooltip={t('setAnOpenerTip')}>
              {t('setAnOpener')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                dir={getDirAttribute(prologueValue || '')}
              ></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <SwitchFormField
        name={'prompt_config.quote'}
        label={t('quote')}
        tooltip={t('quoteTip')}
      ></SwitchFormField>
      <SwitchFormField
        name={'prompt_config.keyword'}
        label={t('keyword')}
        tooltip={t('keywordTip')}
      ></SwitchFormField>
      <SwitchFormField
        name={'prompt_config.tts'}
        label={t('tts')}
        tooltip={t('ttsTip')}
      ></SwitchFormField>
      <TOCEnhanceFormField name="prompt_config.toc_enhance"></TOCEnhanceFormField>
      <TavilyFormField></TavilyFormField>
      <KnowledgeBaseFormField></KnowledgeBaseFormField>
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
                      [],
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
                  defaultValue={field.value || []}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </div>
  );
}

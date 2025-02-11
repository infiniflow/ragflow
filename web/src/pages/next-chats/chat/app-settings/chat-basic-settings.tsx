'use client';

import { FileUploader } from '@/components/file-uploader';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { SwitchFormField } from '@/components/switch-fom-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import { Subhead } from './subhead';

export default function ChatBasicSetting() {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  return (
    <section>
      <Subhead>Basic settings</Subhead>
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
                    // progresses={progresses}
                    // pass the onUpload function here for direct upload
                    // onUpload={uploadFiles}
                    // disabled={isUploading}
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
                <Input {...field}></Input>
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
                <Input {...field}></Input>
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
                <Input {...field}></Input>
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
        <KnowledgeBaseFormField></KnowledgeBaseFormField>
      </div>
    </section>
  );
}

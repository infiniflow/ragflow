'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { FileUploader } from '@/components/file-uploader';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { Subhead } from './subhead';
import { SwitchFormField } from './switch-fom-field';

export default function ChatBasicSetting() {
  const { t } = useTranslate('chat');

  const promptConfigSchema = z.object({
    quote: z.boolean(),
    keyword: z.boolean(),
    tts: z.boolean(),
    empty_response: z.string().min(1, {
      message: t('emptyResponse'),
    }),
    prologue: z.string().min(2, {}),
  });

  const formSchema = z.object({
    name: z.string().min(1, { message: t('assistantNameMessage') }),
    icon: z.array(z.instanceof(File)),
    language: z.string().min(1, {
      message: 'Username must be at least 2 characters.',
    }),
    description: z.string(),
    kb_ids: z.array(z.string()).min(0, {
      message: 'Username must be at least 1 characters.',
    }),
    prompt_config: promptConfigSchema,
  });

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: '',
      language: 'English',
      prompt_config: {
        quote: true,
        keyword: false,
        tts: false,
      },
    },
  });

  function onSubmit(values: z.infer<typeof formSchema>) {
    console.log(values);
  }

  return (
    <section>
      <Subhead>Basic settings</Subhead>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
          <FormField
            control={form.control}
            name="icon"
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
        </form>
      </Form>
    </section>
  );
}

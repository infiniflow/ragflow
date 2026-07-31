'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useFetchLangfuseConfig } from '@/hooks/use-user-setting-request';
import { IModalProps } from '@/interfaces/common';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

export const FormId = 'LangfuseConfigurationForm';

export function LangfuseConfigurationForm({ onOk }: IModalProps<any>) {
  const { t } = useTranslation();
  const { data } = useFetchLangfuseConfig();

  const FormSchema = z.object({
    secret_key: z
      .string()
      .min(1, {
        message: t('setting.secretKeyMessage'),
      })
      .trim(),
    public_key: z
      .string()
      .min(1, {
        message: t('setting.publicKeyMessage'),
      })
      .trim(),
    host: z
      .string()
      .min(0, {
        message: t('setting.hostMessage'),
      })
      .trim(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {},
  });

  async function onSubmit(data: z.infer<typeof FormSchema>) {
    onOk?.(data);
  }

  useEffect(() => {
    if (data) {
      form.reset(data);
    }
  }, [data, form]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={FormId}
      >
        <FormField
          control={form.control}
          name="secret_key"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('setting.secretKey')}</FormLabel>
              <FormControl>
                <Input
                  type={'password'}
                  placeholder={t('setting.secretKeyMessage')}
                  {...field}
                  autoComplete="off"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="public_key"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('setting.publicKey')}</FormLabel>
              <FormControl>
                <Input
                  type={'password'}
                  placeholder={t('setting.publicKeyMessage')}
                  {...field}
                  autoComplete="off"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="host"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Host</FormLabel>
              <FormControl>
                <Input
                  placeholder={'https://cloud.langfuse.com'}
                  {...field}
                  autoComplete="off"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

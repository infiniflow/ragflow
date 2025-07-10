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
import { RAGFlowSelect } from '@/components/ui/select';
import { IModalProps } from '@/interfaces/common';
import { buildOptions } from '@/utils/form';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

export const FormId = 'EditMcpForm';

enum ServerType {
  SSE = 'sse',
  StreamableHttp = 'streamable-http',
}

const ServerTypeOptions = buildOptions(ServerType);

export function useBuildFormSchema() {
  const { t } = useTranslation();

  const FormSchema = z.object({
    name: z
      .string()
      .min(1, {
        message: t('common.namePlaceholder'),
      })
      .trim(),
    url: z
      .string()
      .min(1, {
        message: t('common.namePlaceholder'),
      })
      .trim(),
    server_type: z
      .string()
      .min(1, {
        message: t('common.namePlaceholder'),
      })
      .trim(),
    variables: z.object({}).optional(),
  });

  return FormSchema;
}

export function EditMcpForm({
  initialName,
  onOk,
}: IModalProps<any> & { initialName?: string }) {
  const { t } = useTranslation();

  const FormSchema = useBuildFormSchema();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', server_type: ServerType.SSE, url: '' },
  });

  function onSubmit(data: z.infer<typeof FormSchema>) {
    onOk?.(data);
  }

  useEffect(() => {
    if (initialName) {
      form.setValue('name', initialName);
    }
  }, [form, initialName]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={FormId}
      >
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('common.name')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.namePlaceholder')}
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
          name="url"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('common.url')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.namePlaceholder')}
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
          name="server_type"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('common.serverType')}</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  autoComplete="off"
                  options={ServerTypeOptions}
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

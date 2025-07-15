'use client';

import { UseFormReturn } from 'react-hook-form';
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
import { Editor, loader } from '@monaco-editor/react';
import { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';

loader.config({ paths: { vs: '/vs' } });

export const FormId = 'EditMcpForm';

export enum ServerType {
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
      .url()
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
    headers: z.record(z.string(), z.any()).optional(),
  });

  return FormSchema;
}

export function EditMcpForm({
  form,
  onOk,
  setFieldChanged,
}: IModalProps<any> & {
  form: UseFormReturn<any>;
  setFieldChanged: Dispatch<SetStateAction<boolean>>;
}) {
  const { t } = useTranslation();
  const FormSchema = useBuildFormSchema();

  function onSubmit(data: z.infer<typeof FormSchema>) {
    onOk?.(data);
  }

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
              <FormLabel>{t('mcp.url')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.namePlaceholder')}
                  {...field}
                  autoComplete="off"
                  onChange={(e) => {
                    field.onChange(e.target.value.trim());
                    setFieldChanged(true);
                  }}
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
              <FormLabel>{t('mcp.serverType')}</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  autoComplete="off"
                  options={ServerTypeOptions}
                  onChange={(value) => {
                    field.onChange(value);
                    setFieldChanged(true);
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="headers"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Headers</FormLabel>
              <FormControl>
                <Editor
                  height={200}
                  defaultLanguage="json"
                  theme="vs-dark"
                  {...field}
                  onChange={(value) => {
                    field.onChange(value);
                    setFieldChanged(true);
                  }}
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

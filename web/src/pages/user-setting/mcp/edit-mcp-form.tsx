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
import { loader } from '@monaco-editor/react';
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
        message: t('common.mcp.namePlaceholder'),
      })
      .regex(/^[a-zA-Z0-9_-]{1,64}$/, {
        message: t('common.mcp.nameRequired'),
      })
      .trim(),
    url: z
      .string()
      .url()
      .min(1, {
        message: t('common.mcp.urlPlaceholder'),
      })
      .trim(),
    server_type: z
      .string()
      .min(1, {
        message: t('common.pleaseSelect'),
      })
      .trim(),
    authorization_token: z.string().optional(),
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
              <FormLabel required>{t('common.name')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.mcp.namePlaceholder')}
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
              <FormLabel required>{t('mcp.url')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.mcp.urlPlaceholder')}
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
              <FormLabel required>{t('mcp.serverType')}</FormLabel>
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
          name="authorization_token"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Authorization Token</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.mcp.tokenPlaceholder')}
                  {...field}
                  autoComplete="off"
                  type="password"
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
      </form>
    </Form>
  );
}

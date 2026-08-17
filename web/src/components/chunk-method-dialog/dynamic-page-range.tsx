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

import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { LucidePlus, LucideTrash2 } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Separator } from '../ui/separator';

export function DynamicPageRange() {
  const { t } = useTranslation();
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: 'parser_config.pages',
    control: form.control,
  });

  return (
    <div>
      <FormLabel tooltip={t('knowledgeDetails.pageRangesTip')}>
        {t('knowledgeDetails.pageRanges')}
      </FormLabel>
      {fields.map((field, index) => {
        const typeField = `parser_config.pages.${index}.from`;
        return (
          <div key={field.id} className="flex items-center gap-2 pt-2">
            <FormField
              control={form.control}
              name={typeField}
              render={({ field }) => (
                <FormItem className="w-2/5">
                  <FormDescription />
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={t('common.pleaseInput')}
                      className="!m-0"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <Separator className="w-3 "></Separator>

            <FormField
              control={form.control}
              name={`parser_config.pages.${index}.to`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormDescription />
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={t('common.pleaseInput')}
                      className="!m-0"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <Button
              className="ml-4"
              size="icon"
              variant="outline"
              onClick={() => remove(index)}
            >
              <LucideTrash2 />
            </Button>
          </div>
        );
      })}

      <Button
        onClick={() => append({ from: 1, to: 100 })}
        block
        className="mt-4"
        variant="dashed"
        type="button"
      >
        <LucidePlus />
        {t('knowledgeDetails.addPage')}
      </Button>
    </div>
  );
}

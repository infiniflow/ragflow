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

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { LlmModelType } from '@/constants/knowledge';
import { cn } from '@/lib/utils';
import { Funnel } from 'lucide-react';
import { useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { NextInnerLLMSelectProps, NextLLMSelect } from './llm-select/next';
import { Button } from './ui/button';

export const LargeModelFilterFormSchema = {
  llm_filter: z.string().optional(),
};

type LargeModelFormFieldProps = Pick<
  NextInnerLLMSelectProps,
  'ownerTenantId'
> & {
  name?: string;
};
export function LargeModelFormField({
  ownerTenantId,
  name = 'llm_id',
}: LargeModelFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();
  const filter = useWatch({ control: form.control, name: 'llm_filter' });

  const modelTypes = useMemo(
    () => [
      {
        title: t('flow.allModels'),
        value: 'all',
      },
      {
        title: t('flow.textOnlyModels'),
        value: LlmModelType.Chat,
      },
      {
        title: t('flow.multimodalModels'),
        value: LlmModelType.Image2text,
      },
    ],
    [t],
  );

  return (
    <>
      <FormField
        control={form.control}
        name={name}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('chat.modelTip')}>
              {t('chat.model')}
            </FormLabel>
            <section className="flex gap-2.5">
              <FormField
                control={form.control}
                name="llm_filter"
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant={'ghost'}>
                            <Funnel
                              className={cn(
                                filter && filter !== 'all'
                                  ? 'text-accent-primary'
                                  : 'text-text-disabled',
                              )}
                            />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent>
                          <DropdownMenuRadioGroup
                            value={field.value || 'all'}
                            onValueChange={field.onChange}
                          >
                            {modelTypes.map((x) => (
                              <DropdownMenuRadioItem
                                key={x.value}
                                value={x.value}
                              >
                                {x.title}
                              </DropdownMenuRadioItem>
                            ))}
                          </DropdownMenuRadioGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormControl>
                <NextLLMSelect
                  {...field}
                  filter={filter}
                  ownerTenantId={ownerTenantId}
                />
              </FormControl>
            </section>

            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

type LargeModelFormFieldWithoutFilterProps = Pick<
  NextInnerLLMSelectProps,
  'triggerTestId' | 'optionTestIdPrefix' | 'ownerTenantId'
>;

export function LargeModelFormFieldWithoutFilter({
  triggerTestId,
  optionTestIdPrefix,
  ownerTenantId,
}: LargeModelFormFieldWithoutFilterProps = {}) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name="llm_id"
      render={({ field }) => (
        <FormItem>
          <FormControl>
            <NextLLMSelect
              {...field}
              triggerTestId={triggerTestId}
              optionTestIdPrefix={optionTestIdPrefix}
              ownerTenantId={ownerTenantId}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

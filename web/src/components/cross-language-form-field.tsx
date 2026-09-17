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
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { cn } from '@/lib/utils';
import { toLower } from 'lodash';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export const Languages = [
  'English',
  'Chinese',
  'Spanish',
  'French',
  'German',
  'Japanese',
  'Korean',
  'Vietnamese',
  'Arabic',
  'Turkish',
  'Dutch',
];

export function useCrossLanguageOptions() {
  const { t } = useTranslation();

  return useMemo(
    () =>
      Languages.map((x) => ({
        label: t('language.' + toLower(x)),
        value: x,
      })),
    [t],
  );
}

type CrossLanguageItemProps = {
  name?: string;
  vertical?: boolean;
  label?: string;
};

export const CrossLanguageFormField = ({
  name = 'prompt_config.cross_languages',
  vertical = true,
  label,
}: CrossLanguageItemProps) => {
  const { t } = useTranslation();
  const form = useFormContext();
  const crossLanguageOptions = useCrossLanguageOptions();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn('flex', {
            'gap-2': vertical,
            'flex-col': vertical,
            'justify-between': !vertical,
            'items-center': !vertical,
          })}
        >
          <FormLabel tooltip={t('chat.crossLanguageTip')}>
            {label || t('chat.crossLanguage')}
          </FormLabel>
          <FormControl>
            <MultiSelect
              options={crossLanguageOptions}
              placeholder={t('chat.crossLanguagePlaceholder')}
              maxCount={100}
              {...field}
              onValueChange={field.onChange}
              defaultValue={field.value}
              modalPopover
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
};

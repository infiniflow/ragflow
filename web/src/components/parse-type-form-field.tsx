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
  FormMessage,
} from '@/components/ui/form';
import { Radio } from '@/components/ui/radio';
import { ParseType } from '@/constants/knowledge';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function ParseTypeItem({
  line = 2,
  name = 'parseType',
  builtInLabelKey = 'knowledgeConfiguration.builtIn',
  pipelineLabelKey = 'knowledgeConfiguration.manualSetup',
}: {
  line?: number;
  name?: string;
  builtInLabelKey?: string;
  pipelineLabelKey?: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem className="items-center space-y-0">
          <div
            className={cn('flex', {
              'items-center': line === 1,
              'flex-col gap-1': line === 2,
            })}
          >
            <FormLabel
              className={cn('text-sm  whitespace-wrap ', {
                'w-1/4': line === 1,
              })}
            >
              {t('knowledgeConfiguration.parseType')}
            </FormLabel>
            <div
              className={cn('text-muted-foreground', { 'w-3/4': line === 1 })}
            >
              <FormControl>
                <Radio.Group {...field}>
                  <div
                    className="flex w-full gap-8 text-muted-foreground"
                  >
                    <Radio value={ParseType.BuiltIn}>
                      <span className="whitespace-nowrap">
                        {t(builtInLabelKey)}
                      </span>
                    </Radio>
                    <Radio value={ParseType.Pipeline}>
                      <span className="whitespace-nowrap">
                        {t(pipelineLabelKey)}
                      </span>
                    </Radio>
                  </div>
                </Radio.Group>
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className={line === 1 ? 'w-1/4' : ''}></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}

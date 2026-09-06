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

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Operator } from '@/constants/agent';
import { initialCompilationValues } from '@/pages/agent/constant/pipeline';
import { cn } from '@/lib/utils';
import {
  BuiltinCompilerOperatorId,
  findCompilerOperatorIds,
  hasCompilerOperatorConfig,
} from '@/utils/pipeline-operator';
import { cloneDeep } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const WikiOperatorValue = Operator.Compiler;

export function OptionalWikiOperatorItem({ line = 1 }: { line?: 1 | 2 }) {
  const { t } = useTranslation();
  const form = useFormContext();
  const parserConfig = form.watch('parser_config') as Record<string, any>;
  const hasWikiOperator = hasCompilerOperatorConfig(parserConfig);

  const options = useMemo(
    () => [
      {
        label: t('knowledgeConfiguration.wikiOperator'),
        value: WikiOperatorValue,
      },
    ],
    [t],
  );

  const handleAdd = useCallback(
    (value: string) => {
      if (value !== WikiOperatorValue) {
        return;
      }
      const current = form.getValues('parser_config') || {};
      if (hasCompilerOperatorConfig(current)) {
        return;
      }
      form.setValue('parser_config', {
        ...current,
        [BuiltinCompilerOperatorId]: cloneDeep(initialCompilationValues),
      });
    },
    [form],
  );

  const handleRemove = useCallback(() => {
    const current = { ...(form.getValues('parser_config') || {}) };
    for (const operatorId of findCompilerOperatorIds(current)) {
      delete current[operatorId];
    }
    form.setValue('parser_config', current);
  }, [form]);

  return (
    <div className="items-center space-y-0">
      <div
        className={cn('flex', {
          'items-center': line === 1,
          'flex-col gap-1': line === 2,
        })}
      >
        <FormLabel
          className={cn('text-sm whitespace-wrap', {
            'w-1/4': line === 1,
          })}
        >
          {t('knowledgeConfiguration.wikiOperatorLabel')}
        </FormLabel>
        <div className={cn('text-muted-foreground', { 'w-3/4': line === 1 })}>
          {hasWikiOperator ? (
            <div className="flex items-center gap-2">
              <span className="text-sm text-text-primary">
                {t('knowledgeConfiguration.wikiOperator')}
              </span>
              <Button
                type="button"
                variant="transparent"
                size="sm"
                onClick={handleRemove}
              >
                {t('common.remove')}
              </Button>
            </div>
          ) : (
            <SelectWithSearch
              value=""
              onChange={handleAdd}
              placeholder={t('knowledgeConfiguration.wikiOperatorPlaceholder')}
              options={options}
            />
          )}
        </div>
      </div>
      <p
        className={cn('text-xs text-text-secondary pt-1', {
          'pl-[25%]': line === 1,
        })}
      >
        {t('knowledgeConfiguration.wikiOperatorTip')}
      </p>
    </div>
  );
}

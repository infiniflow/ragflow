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

import { LlmModelType } from '@/constants/knowledge';
import { useModelValidIds } from '@/hooks/use-llm-request';
import * as SelectPrimitive from '@radix-ui/react-select';
import { forwardRef, memo, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LlmSettingFieldItems } from '../llm-setting-items/next';
import { ModelTypeMap } from '../model-tree-select';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Select, SelectTrigger, SelectValue } from '../ui/select';
import LLMLabel, { MissingModelLabel } from './llm-label';

export interface NextInnerLLMSelectProps {
  id?: string;
  value?: string;
  onInitialValue?: (value: string, option: any) => void;
  onChange?: (value: string) => void;
  disabled?: boolean;
  filter?: string;
  triggerTestId?: string;
  optionTestIdPrefix?: string;
  ownerTenantId?: string;
}

const NextInnerLLMSelect = forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  NextInnerLLMSelectProps
>(
  (
    {
      value,
      disabled,
      filter,
      triggerTestId,
      optionTestIdPrefix,
      ownerTenantId,
    },
    ref,
  ) => {
    const { t } = useTranslation();
    const [isPopoverOpen, setIsPopoverOpen] = useState(false);

    const modelTypes = useMemo(() => {
      if (filter === LlmModelType.Chat) {
        return ['chat'];
      } else if (filter === LlmModelType.Image2text) {
        return ['vision'];
      } else {
        return ['chat', 'vision'];
      }
    }, [filter]);

    // Validity is checked against the current user's own models — runs
    // resolve llm_id against the runner's tenant, so a model only the
    // canvas owner has added is unusable. Gated on isFetched so a slow
    // list never flashes a false missing state. The filter-derived
    // modelTypes only narrow the dropdown display, not validity.
    const { validIds, isFetched: ownModelsFetched } = useModelValidIds(
      ModelTypeMap.llm_id,
    );
    const isModelMissing = !!value && ownModelsFetched && !validIds.has(value);

    return (
      <Select disabled={disabled} value={value}>
        <Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
          <PopoverTrigger asChild>
            <SelectTrigger
              onClick={(e) => {
                e.preventDefault();
                setIsPopoverOpen(true);
              }}
              ref={ref}
              data-testid={triggerTestId}
            >
              <SelectValue placeholder={t('common.pleaseSelect')}>
                {isModelMissing ? (
                  <MissingModelLabel
                    value={value}
                    ownerTenantId={ownerTenantId}
                  />
                ) : (
                  <LLMLabel value={value} ownerTenantId={ownerTenantId} />
                )}
              </SelectValue>
            </SelectTrigger>
          </PopoverTrigger>
          <PopoverContent side={'left'}>
            <LlmSettingFieldItems
              modelTypes={modelTypes}
              llmOptionTestIdPrefix={optionTestIdPrefix}
              ownerTenantId={ownerTenantId}
            ></LlmSettingFieldItems>
          </PopoverContent>
        </Popover>
      </Select>
    );
  },
);

NextInnerLLMSelect.displayName = 'LLMSelect';

export const NextLLMSelect = memo(NextInnerLLMSelect);

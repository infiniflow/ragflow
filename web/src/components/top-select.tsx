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

import { forwardRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { FormLayout } from '@/constants/form';
import { SelectWithSearch } from './originui/select-with-search';
import { SliderInputFormField } from './slider-input-form-field';

type TopSelectProps = {
  max?: number;
  value?: number;
  onChange?(value: number): void;
};

export const TopSelect = forwardRef<HTMLButtonElement, TopSelectProps>(
  function TopSelect({ max = 100, value = 10, onChange }, ref) {
    const { t } = useTranslation();

    const sizeChangerOptions = useMemo(() => {
      return [10, 20, 50, 100]
        .filter((x) => x <= max)
        .map((x) => ({
          label: <span>{t('common.top', { top: x })}</span>,
          value: x.toString(),
        }));
    }, [max, t]);

    return (
      <SelectWithSearch
        ref={ref}
        options={sizeChangerOptions}
        value={value.toString()}
        onChange={(val) => onChange?.(Number(val))}
      ></SelectWithSearch>
    );
  },
);

export function TopSelectFormItem() {
  const { t } = useTranslation();

  return (
    <SliderInputFormField
      name="size"
      label={t('chat.topN')}
      min={1}
      max={100}
      layout={FormLayout.Vertical}
    ></SliderInputFormField>
  );
}

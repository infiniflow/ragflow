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
import { SelectWithSearch } from './originui/select-with-search';
import { RAGFlowFormItem } from './ragflow-form';

type TopSelectProps = {
  value?: number;
  onChange?(value: number): void;
};

export const TopSelect = forwardRef<HTMLButtonElement, TopSelectProps>(
  function TopSelect({ value = 10, onChange }, ref) {
    const { t } = useTranslation();

    const sizeChangerOptions = useMemo(() => {
      return [10, 20, 50, 100].map((x) => ({
        label: <span>{t('common.top', { top: x })}</span>,
        value: x.toString(),
      }));
    }, [t]);

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
    <RAGFlowFormItem label={t('knowledgeConfiguration.top')} name="size">
      <TopSelect></TopSelect>
    </RAGFlowFormItem>
  );
}

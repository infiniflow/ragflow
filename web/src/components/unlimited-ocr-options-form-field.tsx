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

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SelectWithSearch } from './originui/select-with-search';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';

const CropModeOptions = buildOptions(['auto', 'gundam', 'base']);

/** Edits Unlimited-OCR-2 options under the parent-selected setup. */
export function UnlimitedOCROptionsFormField({
  namePrefix,
}: {
  namePrefix: string;
}) {
  const { t } = useTranslation();
  const buildName = (field: string) =>
    namePrefix ? `${namePrefix}.${field}` : field;

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        Unlimited-OCR-2 Options
      </div>

      <RAGFlowFormItem
        name={buildName('unlimited_ocr_crop_mode')}
        label="Crop Mode"
        tooltip="Inference crop mode: auto (Gundam for 1-2 pages, Base for multi-page), gundam (always tile for high detail), base (single pass linear complexity)."
        horizontal={true}
      >
        {(field) => (
          <SelectWithSearch
            value={field.value || 'auto'}
            onChange={field.onChange}
            options={CropModeOptions}
            placeholder={t('common.selectPlaceholder', 'Select value')}
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('unlimited_ocr_no_repeat_ngram_size')}
        label="No Repeat N-Gram Size"
        tooltip="Repetition suppression sequence window (default 35). Prevents degenerative loops on large tables or borders."
        horizontal={true}
      >
        {(field) => (
          <Input
            type="number"
            {...field}
            value={field.value ?? 35}
            onChange={(e) => {
              const val = e.target.value.trim();
              if (val === '') {
                field.onChange(undefined);
              } else {
                const parsed = Number(val);
                field.onChange(Number.isNaN(parsed) ? undefined : parsed);
              }
            }}
            placeholder="35"
          />
        )}
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name={buildName('unlimited_ocr_page_guard')}
        label="Page Count Guard"
        tooltip="Enforces page count synchronization between visual markers and generated tokens."
        horizontal={true}
        labelClassName="!mb-0 whitespace-nowrap"
        valueClassName="w-auto shrink-0"
      >
        {(field) => (
          <Switch
            checked={field.value ?? true}
            onCheckedChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>
    </div>
  );
}

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

import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import {
  LayoutRecognizeFormField,
  ParseDocumentType,
} from './layout-recognize-form-field';
import { RAGFlowFormItem } from './ragflow-form';
import { Input } from './ui/input';

const builtInParserOptions = [
  ParseDocumentType.DeepDOC,
  ParseDocumentType.PlainText,
  ParseDocumentType.Docling,
  ParseDocumentType.OpenDataLoader,
  ParseDocumentType.TCADPParser,
  ParseDocumentType.MonkeyOCRv2,
].map((value) => ({ value, label: value }));

export function AutoLayoutRecognizeFormFields({
  layoutRecognizeName = 'parser_config.layout_recognize',
  namePrefix = 'parser_config',
  horizontal = true,
  ownerTenantId,
}: {
  layoutRecognizeName?: string;
  namePrefix?: string;
  horizontal?: boolean;
  ownerTenantId?: string;
}) {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const layoutRecognize = useWatch({
    control: form.control,
    name: layoutRecognizeName,
  });

  const isAuto =
    typeof layoutRecognize === 'string' &&
    layoutRecognize.trim().toLowerCase() === 'auto';

  const subParserOptions = useMemo(() => builtInParserOptions, []);

  if (!isAuto) {
    return null;
  }

  const buildName = (field: string) =>
    namePrefix ? `${namePrefix}.${field}` : field;

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        {t('layoutRecognizeAutoTitle')}
      </div>
      <LayoutRecognizeFormField
        name={buildName('layout_recognize_auto_text')}
        horizontal={horizontal}
        optionsWithoutLLM={subParserOptions}
        label={t('layoutRecognizeAutoText')}
        showMineruOptions={false}
        showPaddleocrOptions={false}
        ownerTenantId={ownerTenantId}
      />
      <LayoutRecognizeFormField
        name={buildName('layout_recognize_auto_scanned')}
        horizontal={horizontal}
        label={t('layoutRecognizeAutoScanned')}
        showMineruOptions={false}
        showPaddleocrOptions={false}
        ownerTenantId={ownerTenantId}
        omitParserValues={[ParseDocumentType.Auto]}
      />
      <RAGFlowFormItem
        name={buildName('layout_recognize_auto_min_chars_per_page')}
        label={t('layoutRecognizeAutoMinChars')}
        tooltip={t('layoutRecognizeAutoMinCharsTip')}
      >
        {(field) => (
          <Input
            type="number"
            min={1}
            {...field}
            value={field.value ?? 100}
            onChange={(e) => field.onChange(Number(e.target.value))}
          />
        )}
      </RAGFlowFormItem>
    </div>
  );
}

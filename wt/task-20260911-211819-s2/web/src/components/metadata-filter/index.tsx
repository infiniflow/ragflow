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

import { DatasetMetadata } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { SelectWithSearch } from '../originui/select-with-search';
import { RAGFlowFormItem } from '../ragflow-form';
import { MetadataFilterConditions } from './metadata-filter-conditions';
import { MetadataSemiAutoFields } from './metadata-semi-auto-fields';

type MetadataFilterProps = {
  prefix?: string;
  canReference?: boolean;
};

export const MetadataFilterSchema = {
  meta_data_filter: z
    .object({
      logic: z.string().optional(),
      method: z.string().optional(),
      manual: z
        .array(
          z.object({
            key: z.string(),
            op: z.string(),
            value: z.union([z.string(), z.array(z.string())]),
          }),
        )
        .optional(),
      semi_auto: z
        .array(
          z.union([
            z.string(),
            z.object({
              key: z.string(),
              op: z.string().optional(),
            }),
          ]),
        )
        .optional(),
    })
    .optional(),
};

export function MetadataFilter({
  prefix = '',
  canReference,
}: MetadataFilterProps) {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  const methodName = prefix + 'meta_data_filter.method';

  const datasetIds: string[] = useWatch({
    control: form.control,
    name: prefix + 'dataset_ids',
  });

  const oldKbIds: string[] = useWatch({
    control: form.control,
    name: prefix + 'kb_ids',
  });

  const kbIds = datasetIds || oldKbIds || [];

  const metadata = useWatch({
    control: form.control,
    name: methodName,
  });
  const hasKnowledge = Array.isArray(kbIds) && kbIds.length > 0;

  const MetadataOptions = Object.values(DatasetMetadata).map((x) => {
    return {
      value: x,
      label: t(`meta.${x}`),
    };
  });

  return (
    <>
      {hasKnowledge && (
        <RAGFlowFormItem
          label={t('metadata')}
          name={methodName}
          tooltip={t('metadataTip')}
        >
          <SelectWithSearch
            options={MetadataOptions}
            triggerClassName="!bg-bg-input"
          />
        </RAGFlowFormItem>
      )}
      {hasKnowledge && metadata === DatasetMetadata.Manual && (
        <MetadataFilterConditions
          kbIds={kbIds}
          prefix={prefix}
          canReference={canReference}
        ></MetadataFilterConditions>
      )}
      {hasKnowledge && metadata === DatasetMetadata.SemiAutomatic && (
        <MetadataSemiAutoFields
          kbIds={kbIds}
          prefix={prefix}
        ></MetadataSemiAutoFields>
      )}
    </>
  );
}

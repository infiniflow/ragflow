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

import { useFetchAllMemoryList } from '@/hooks/use-memory-request';
import { IMemory } from '@/interfaces/database/memory';
import { useMemo, useRef } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { RAGFlowAvatar } from './ragflow-avatar';
import { RAGFlowFormItem } from './ragflow-form';
import { MultiSelect } from './ui/multi-select';

type MemoriesFormFieldProps = {
  label: string;
  name?: string;
  required?: boolean;
};

function MemoryLabel({ text }: { text: string }) {
  return (
    <div className="text-xs px-3 p-1 bg-bg-card text-text-secondary rounded-lg border border-bg-card">
      {text}
    </div>
  );
}

export function useDisableDifferenceEmbeddingMemory(name: string) {
  const form = useFormContext();
  const memoryIds = useWatch({ name, control: form.control });
  const { data: memoryListOrigin } = useFetchAllMemoryList();
  const memoryCacheRef = useRef(new Map<string, IMemory>());

  const memoryList = useMemo(() => {
    memoryListOrigin?.forEach((memory) => {
      memoryCacheRef.current.set(memory.id, memory);
    });

    const selectedIds = Array.isArray(memoryIds) ? memoryIds : [];
    const selectedMemories = selectedIds
      .map((id) => memoryCacheRef.current.get(id))
      .filter(Boolean) as IMemory[];

    return Array.from(
      new Map(
        [...(memoryListOrigin ?? []), ...selectedMemories].map((memory) => [
          memory.id,
          memory,
        ]),
      ).values(),
    );
  }, [memoryIds, memoryListOrigin]);

  const selectedEmbedId = useMemo(() => {
    const data = memoryList?.find((item) => item.id === memoryIds?.[0]);
    return data?.embd_id ?? '';
  }, [memoryIds, memoryList]);

  const options = useMemo(() => {
    return memoryList.filter(Boolean).map((item: IMemory) => {
      return {
        label: item.name,
        icon: () => (
          <RAGFlowAvatar
            className="size-4"
            avatar={item.avatar ?? ''}
            name={item.name}
          />
        ),
        suffix: <MemoryLabel text={item.embd_name || item.embd_id} />,
        value: item.id,
        disabled: item.embd_id !== selectedEmbedId && selectedEmbedId !== '',
      };
    });
  }, [memoryList, selectedEmbedId]);

  return {
    options,
  };
}

export function MemoriesFormField({
  label,
  name = 'memory_ids',
  required = false,
}: MemoriesFormFieldProps) {
  const { t } = useTranslation();
  const { options } = useDisableDifferenceEmbeddingMemory(name);

  return (
    <RAGFlowFormItem name={name} label={label} required={required}>
      {(field) => (
        <MultiSelect
          options={options}
          placeholder={t('common.pleaseSelect')}
          maxCount={100}
          onValueChange={field.onChange}
          defaultValue={field.value}
          showSelectAll={false}
          modalPopover
        />
      )}
    </RAGFlowFormItem>
  );
}

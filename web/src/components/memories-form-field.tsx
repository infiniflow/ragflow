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
    return memoryList
      .filter(Boolean)
      .map((item: IMemory) => {
        return {
          label: item.name,
          icon: () => (
            <RAGFlowAvatar
              className="size-4"
              avatar={item.avatar ?? ''}
              name={item.name}
            />
          ),
          suffix: <MemoryLabel text={item.embd_id} />,
          value: item.id,
          disabled:
            item.embd_id !== selectedEmbedId && selectedEmbedId !== '',
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
}: MemoriesFormFieldProps) {
  const { t } = useTranslation();
  const { options } = useDisableDifferenceEmbeddingMemory(name);

  return (
    <RAGFlowFormItem name={name} label={label}>
      {(field) => (
        <MultiSelect
          options={options}
          placeholder={t('common.pleaseSelect')}
          maxCount={100}
          onValueChange={field.onChange}
          defaultValue={field.value}
          modalPopover
        />
      )}
    </RAGFlowFormItem>
  );
}

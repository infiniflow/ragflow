import { useFetchAllMemoryList } from '@/hooks/use-memory-request';
import { useTranslation } from 'react-i18next';
import { RAGFlowFormItem } from './ragflow-form';
import { MultiSelect } from './ui/multi-select';

type MemoriesFormFieldProps = {
  label: string;
};

export function MemoriesFormField({ label }: MemoriesFormFieldProps) {
  const { t } = useTranslation();
  const memoryList = useFetchAllMemoryList();

  const options = memoryList.data?.map((memory) => ({
    label: memory.name,
    value: memory.id,
  }));

  return (
    <RAGFlowFormItem name="memory_ids" label={label}>
      {(field) => (
        <MultiSelect
          options={options || []}
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

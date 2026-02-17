import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import {
  EmbeddingSelect,
  LLMSelect,
} from '@/pages/dataset/dataset-setting/configuration/common-item';
import { TFunction } from 'i18next';
export enum MemoryType {
  Raw = 'raw',
  Semantic = 'semantic',
  Episodic = 'episodic',
  Procedural = 'procedural',
}
export const MemoryOptions = (t: TFunction) => [
  { label: t('memories.raw'), value: MemoryType.Raw },
  { label: t('memories.semantic'), value: MemoryType.Semantic },
  { label: t('memories.episodic'), value: MemoryType.Episodic },
  { label: t('memories.procedural'), value: MemoryType.Procedural },
];
export const createMemoryFields = (t: TFunction) =>
  [
    {
      name: 'name',
      label: t('memories.name'),
      placeholder: t('memories.memoryNamePlaceholder'),
      required: true,
    },
    {
      name: 'memory_type',
      label: t('memories.memoryType'),
      type: FormFieldType.MultiSelect,
      placeholder: t('memories.descriptionPlaceholder'),
      tooltip: t('memories.memoryTypeTooltip'),
      options: MemoryOptions(t),
      required: true,
      customValidate: (value) => {
        if (!value.includes(MemoryType.Raw) || !value.length) {
          return t('memories.embeddingModelError');
        }
        return true;
      },
    },
    {
      name: 'embd_id',
      label: t('memories.embeddingModel'),
      placeholder: t('memories.selectModel'),
      tooltip: t('memories.embeddingModelTooltip'),
      required: true,
      // hideLabel: true,
      // type: 'custom',
      render: (field) => <EmbeddingSelect field={field} isEdit={false} />,
    },
    {
      name: 'llm_id',
      label: t('memories.llm'),
      placeholder: t('memories.selectModel'),
      required: true,
      type: FormFieldType.Select,
      tooltip: t('memories.llmTooltip'),
      render: (field) => <LLMSelect field={field} isEdit={false} />,
    },
  ] as FormFieldConfig[];

export const defaultMemoryFields = {
  name: '',
  memory_type: [MemoryType.Raw],
  embd_id: '',
  llm_id: '',
};

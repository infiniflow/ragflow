import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { EmbeddingSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { t } from 'i18next';

export const createMemoryFields = [
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
    options: [
      { label: 'Raw', value: 'raw' },
      { label: 'Semantic', value: 'semantic' },
      { label: 'Episodic', value: 'episodic' },
      { label: 'Procedural', value: 'procedural' },
    ],
    required: true,
  },
  {
    name: 'embd_id',
    label: t('memories.embeddingModel'),
    placeholder: t('memories.selectModel'),
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
  },
] as FormFieldConfig[];

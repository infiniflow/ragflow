import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { EmbeddingSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { t } from 'i18next';

export const createMemoryFields = [
  {
    name: 'memory_name',
    label: t('memory.name'),
    placeholder: t('memory.memoryNamePlaceholder'),
    required: true,
  },
  {
    name: 'memory_type',
    label: t('memory.memoryType'),
    type: FormFieldType.MultiSelect,
    placeholder: t('memory.descriptionPlaceholder'),
    options: [
      { label: 'Raw', value: 'raw' },
      { label: 'Semantic', value: 'semantic' },
      { label: 'Episodic', value: 'episodic' },
      { label: 'Procedural', value: 'procedural' },
    ],
    required: true,
  },
  {
    name: 'embedding',
    label: t('memory.embeddingModel'),
    placeholder: t('memory.selectModel'),
    required: true,
    // hideLabel: true,
    // type: 'custom',
    render: (field) => <EmbeddingSelect field={field} isEdit={false} />,
  },
  {
    name: 'llm',
    label: t('memory.llm'),
    placeholder: t('memory.selectModel'),
    required: true,
    type: FormFieldType.Select,
  },
] as FormFieldConfig[];

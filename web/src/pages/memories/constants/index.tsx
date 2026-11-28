import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { EmbeddingSelect } from '@/pages/dataset/setting/embedding-select';
import { LLMSelect } from '@/pages/dataset/setting/common-item';
import { TFunction } from 'i18next';
// Memory types are the built-in compilation pipelines. The old
// semantic/episodic/procedural split is gone: all three are one pipeline
// (memory_notes), and memory_facts is the fact graph.
export enum MemoryType {
  Raw = 'raw',
  Notes = 'memory_notes',
  Facts = 'memory_facts',
}
export const MemoryOptions = (t: TFunction) => [
  { label: t('memories.raw'), value: MemoryType.Raw, locked: true },
  { label: t('memories.memoryNotes'), value: MemoryType.Notes },
  { label: t('memories.memoryFacts'), value: MemoryType.Facts },
];
// createMemoryFields no longer asks which memory types to extract: semantic,
// episodic and procedural are built-in pipelines that always run, and raw is
// always stored. The API keeps accepting memory_type, but omitting it now
// means "every built-in pipeline", so the form simply says nothing.
export const createMemoryFields = (t: TFunction) =>
  [
    {
      name: 'name',
      label: t('memories.name'),
      placeholder: t('memories.memoryNamePlaceholder'),
      required: true,
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
  embd_id: '',
  llm_id: '',
};

import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { useModelOptions } from '@/components/llm-setting-items/llm-form-field';
import { EmbeddingSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { t } from 'i18next';
import { z } from 'zod';

export const memoryModelFormSchema = {
  embd_id: z.string(),
  llm_id: z.string(),
  memory_type: z.array(z.string()).optional(),
  memory_size: z.number().optional(),
};
export const defaultMemoryModelForm = {
  embd_id: '',
  llm_id: '',
  memory_type: [],
  memory_size: 0,
};
export const MemoryModelForm = () => {
  const { modelOptions } = useModelOptions();
  return (
    <>
      <RenderField
        field={{
          name: 'embd_id',
          label: t('memories.embeddingModel'),
          placeholder: t('memories.selectModel'),
          required: true,
          horizontal: true,
          // hideLabel: true,
          type: FormFieldType.Custom,
          render: (field) => <EmbeddingSelect field={field} isEdit={false} />,
        }}
      />
      <RenderField
        field={{
          name: 'llm_id',
          label: t('memories.llm'),
          placeholder: t('memories.selectModel'),
          required: true,
          horizontal: true,
          type: FormFieldType.Select,
          options: modelOptions as { value: string; label: string }[],
        }}
      />
      <RenderField
        field={{
          name: 'memory_type',
          label: t('memories.memoryType'),
          type: FormFieldType.MultiSelect,
          horizontal: true,
          placeholder: t('memories.memoryTypePlaceholder'),
          options: [
            { label: 'Raw', value: 'raw' },
            { label: 'Semantic', value: 'semantic' },
            { label: 'Episodic', value: 'episodic' },
            { label: 'Procedural', value: 'procedural' },
          ],
          required: true,
        }}
      />
      <RenderField
        field={{
          name: 'memory_size',
          label: t('memory.config.memorySize'),
          type: FormFieldType.Number,
          horizontal: true,
          // placeholder: t('memory.config.memorySizePlaceholder'),
          required: false,
        }}
      />
    </>
  );
};

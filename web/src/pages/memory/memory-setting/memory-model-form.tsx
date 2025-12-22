import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { useModelOptions } from '@/components/llm-setting-items/llm-form-field';
import { EmbeddingSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { MemoryType } from '@/pages/memories/constants';
import { useTranslation } from 'react-i18next';
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
  memory_type: [MemoryType.Raw],
  memory_size: 0,
};
export const MemoryModelForm = () => {
  const { modelOptions } = useModelOptions();
  const { t } = useTranslation();
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
          disabled: true,
          render: (field) => (
            <EmbeddingSelect field={field} isEdit={false} disabled={true} />
          ),

          tooltip: t('memories.embeddingModelTooltip'),
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
          tooltip: t('memories.llmTooltip'),
        }}
      />
      <RenderField
        field={{
          name: 'memory_type',
          label: t('memories.memoryType'),
          type: FormFieldType.MultiSelect,
          horizontal: true,
          placeholder: t('memories.memoryTypePlaceholder'),
          tooltip: t('memories.memoryTypeTooltip'),
          disabled: true,
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

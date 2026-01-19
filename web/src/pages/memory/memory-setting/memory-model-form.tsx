import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { useModelOptions } from '@/components/llm-setting-items/llm-form-field';
import { EmbeddingSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { MemoryOptions, MemoryType } from '@/pages/memories/constants';
import { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useFetchMemoryMessageList } from '../memory-message/hook';

export const memoryModelFormSchema = (t: TFunction) => ({
  embd_id: z.string(),
  llm_id: z.string(),
  memory_type: z.array(z.string()).superRefine((data, ctx) => {
    if (!data.includes(MemoryType.Raw) || !data.length) {
      ctx.addIssue({
        // path: ['memory_type'],
        message: t('memories.embeddingModelError'),
        code: 'custom',
      });
    }
  }),
  memory_size: z.number().optional(),
});
export const defaultMemoryModelForm = {
  embd_id: '',
  llm_id: '',
  memory_type: [],
  memory_size: 0,
};
export const MemoryModelForm = () => {
  const { modelOptions } = useModelOptions();
  const { t } = useTranslation();
  const { data } = useFetchMemoryMessageList();
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
            <EmbeddingSelect
              field={field}
              isEdit={false}
              disabled={data?.messages?.total_count > 0}
            />
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
          disabled: data?.messages?.total_count > 0,
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
          disabled: data?.messages?.total_count > 0,
          options: MemoryOptions(t),
          customValidate: (value) => {
            if (!value.includes(MemoryType.Raw) || !value.length) {
              return t('memories.embeddingModelError');
            }
            return true;
          },
          required: true,
        }}
      />
      <RenderField
        field={{
          name: 'memory_size',
          label: t('memory.config.memorySize') + ' (Bytes)',
          type: FormFieldType.Number,
          horizontal: true,
          tooltip: t('memory.config.memorySizeTooltip'),
          // placeholder: t('memory.config.memorySizePlaceholder'),
          required: false,
        }}
      />
    </>
  );
};

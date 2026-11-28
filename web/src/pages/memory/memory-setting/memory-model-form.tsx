import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { ModelTreeSelect } from '@/components/model-tree-select';
import { EmbeddingSelect } from '@/pages/dataset/setting/embedding-select';
import { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useFetchMemoryMessageList } from '../memory-message/hook';
import { useMemorySettingContext } from './memory-setting-context';

// Extraction is no longer configured per memory: semantic, episodic and
// procedural are built-in pipelines that always run, so the form only carries
// the models and the capacity.
export const memoryModelFormSchema = (_t: TFunction) => ({
  embd_id: z.string(),
  llm_id: z.string(),
  memory_size: z.number().optional(),
});
export const defaultMemoryModelForm = {
  embd_id: '',
  llm_id: '',
  memory_size: 0,
};
export const MemoryModelForm = ({
  readOnly = false,
}: {
  readOnly?: boolean;
}) => {
  const { t } = useTranslation();
  const { data } = useFetchMemoryMessageList();
  const { data: configData } = useMemorySettingContext();
  const resourceTenantId = configData?.tenant_id;
  const accessUserId = configData?.user_id;
  return (
    <>
      <RenderField
        field={{
          name: 'embd_id',
          label: t('memories.embeddingModel'),
          placeholder: t('memories.selectModel'),
          required: true,
          horizontal: true,
          type: FormFieldType.Custom,
          disabled: true,
          render: (field) => (
            <EmbeddingSelect
              field={field}
              isEdit={false}
              disabled={readOnly || data?.messages?.total_count > 0}
              resourceTenantId={resourceTenantId}
              accessUserId={accessUserId}
            />
          ),

          tooltip: t('memories.embeddingModelTooltip'),
        }}
      />
      <RenderField
        field={{
          name: 'llm_id',
          label: t('memories.llm'),
          required: true,
          horizontal: true,
          type: FormFieldType.Custom,
          disabled: readOnly || data?.messages?.total_count > 0,
          render: (field) => (
            <ModelTreeSelect
              value={field.value}
              onChange={field.onChange}
              disabled={readOnly || data?.messages?.total_count > 0}
              placeholder={t('memories.selectModel')}
              resourceTenantId={resourceTenantId}
              accessUserId={accessUserId}
            />
          ),
          tooltip: t('memories.llmTooltip'),
        }}
      />
      <RenderField
        field={{
          name: 'memory_size',
          label: t('memory.config.memorySize') + ' (Bytes)',
          type: FormFieldType.Number,
          horizontal: true,
          tooltip: t('memory.config.memorySizeTooltip'),
          required: false,
          disabled: readOnly,
        }}
      />
    </>
  );
};

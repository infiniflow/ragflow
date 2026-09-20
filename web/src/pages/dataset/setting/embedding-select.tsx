import { ModelTreeSelect, ModelTypeMap } from '@/components/model-tree-select';
import { Spin } from '@/components/ui/spin';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { useMemo } from 'react';
import { FieldValues, useFormContext } from 'react-hook-form';
import { useCheckKbEmbedding } from './hooks';

export const EmbeddingSelect = ({
  isEdit,
  field,
  name,
  disabled = false,
  testId,
  ownerTenantId,
}: {
  isEdit: boolean;
  field: FieldValues;
  name?: string;
  disabled?: boolean;
  testId?: string;
  ownerTenantId?: string;
}) => {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const { checkKbEmbedding, checking } = useCheckKbEmbedding();

  const oldValue = useMemo(() => {
    const embdStr = form.getValues(name || 'embedding_model');
    return embdStr || '';
  }, [form, name]);
  return (
    <div className="space-y-1.5">
      <ModelTreeSelect
        modelTypes={ModelTypeMap.embd_id}
        onChange={async (value) => {
          field.onChange(value);
          if (isEdit && disabled) {
            const res = await checkKbEmbedding(value);
            if (res?.code !== 0) {
              field.onChange(oldValue);
            }
          }
        }}
        ownerTenantId={ownerTenantId}
        disabled={(disabled && !isEdit) || checking}
        value={field.value}
        placeholder={t('embeddingModelPlaceholder')}
        testId={testId}
        className={cn({ 'opacity-60': checking })}
      />
      {checking && (
        <div className="flex items-center gap-2 text-sm text-text-secondary">
          <Spin size="small" />
          {t('checkingEmbedding')}
        </div>
      )}
    </div>
  );
};

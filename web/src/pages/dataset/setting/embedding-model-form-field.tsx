import { ModelTreeSelect, ModelTypeMap } from '@/components/model-tree-select';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Spin } from '@/components/ui/spin';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { useMemo, useState } from 'react';
import { FieldValues, useFormContext } from 'react-hook-form';
import { useHandleKbEmbedding, useHasParsedDocument } from './hooks';

interface IProps {
  line?: 1 | 2;
  isEdit?: boolean;
}

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
  const { handleChange } = useHandleKbEmbedding();

  const oldValue = useMemo(() => {
    const embdStr = form.getValues(name || 'embedding_model');
    return embdStr || '';
  }, [form, name]);
  const [loading, setLoading] = useState(false);
  return (
    <Spin
      spinning={loading}
      className={cn('rounded-lg after:bg-bg-base', {
        'opacity-20': loading,
      })}
    >
      <ModelTreeSelect
        modelTypes={ModelTypeMap.embd_id}
        onChange={async (value) => {
          field.onChange(value);
          if (isEdit && disabled) {
            setLoading(true);
            const res = await handleChange({
              embed_id: value,
            });
            if (res.code !== 0) {
              field.onChange(oldValue);
            }
            setLoading(false);
          }
        }}
        ownerTenantId={ownerTenantId}
        disabled={disabled && !isEdit}
        value={field.value}
        placeholder={t('embeddingModelPlaceholder')}
        testId={testId}
      />
    </Spin>
  );
};

export function EmbeddingModelItem({
  line = 1,
  isEdit,
  ownerTenantId,
}: IProps & { ownerTenantId?: string }) {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const disabled = useHasParsedDocument(isEdit);
  return (
    <>
      <FormField
        control={form.control}
        name={'embedding_model'}
        render={({ field }) => (
          <FormItem className={cn('items-center space-y-0')}>
            <div
              className={cn('flex', {
                'items-center': line === 1,
                'flex-col gap-1': line === 2,
              })}
            >
              <FormLabel
                required
                tooltip={t('embeddingModelTip')}
                className={cn('text-sm whitespace-wrap', {
                  'w-1/4': line === 1,
                })}
              >
                {t('embeddingModel')}
              </FormLabel>
              <div
                className={cn('text-muted-foreground', { 'w-3/4': line === 1 })}
              >
                <FormControl>
                  <EmbeddingSelect
                    isEdit={!!isEdit}
                    field={field}
                    disabled={disabled}
                    testId="ds-settings-basic-embedding-model-select"
                    ownerTenantId={ownerTenantId}
                  ></EmbeddingSelect>
                </FormControl>
              </div>
            </div>
            <div className="flex pt-1">
              <div className={line === 1 ? 'w-1/4' : ''}></div>
              <FormMessage />
            </div>
          </FormItem>
        )}
      />
    </>
  );
}

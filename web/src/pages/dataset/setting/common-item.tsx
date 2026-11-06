import { ModelTreeSelect, ModelTypeMap } from '@/components/model-tree-select';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchDatasetPipelineConfiguration } from '@/hooks/use-knowledge-request';
import { cn } from '@/lib/utils';
import { FieldValues, useFormContext } from 'react-hook-form';
import { EmbeddingSelect } from './embedding-select';

interface IProps {
  line?: 1 | 2;
  isEdit?: boolean;
  label?: string;
  name?: string;
}

export function EmbeddingModelItem({
  line = 1,
  isEdit,
  ownerTenantId,
}: IProps & { ownerTenantId?: string }) {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const { data: knowledgeDetails } = useFetchDatasetPipelineConfiguration({
    isEdit,
  });
  const disabled = knowledgeDetails.chunk_count > 0;
  return (
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
            <div className={cn('text-text-primary', { 'w-3/4': line === 1 })}>
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
  );
}

export const LLMSelect = ({
  isEdit,
  field,
  disabled = false,
  ownerTenantId,
}: {
  isEdit: boolean;
  field: FieldValues;
  name?: string;
  disabled?: boolean;
  ownerTenantId?: string;
}) => {
  const { t } = useTranslate('knowledgeConfiguration');
  return (
    <ModelTreeSelect
      modelTypes={ModelTypeMap.llm_id}
      onChange={(value) => {
        field.onChange(value);
      }}
      disabled={disabled && !isEdit}
      value={field.value}
      placeholder={t('embeddingModelPlaceholder')}
      ownerTenantId={ownerTenantId}
    />
  );
};

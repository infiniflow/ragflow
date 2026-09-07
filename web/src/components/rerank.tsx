import { ModelTreeSelect } from '@/components/model-tree-select';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

export const topKSchema = {
  top_k: z.number().optional(),
};

export const initialTopKValue = {
  top_k: 1024,
};

const RerankId = 'rerank_id';

function RerankFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name={RerankId}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('rerankTip')}>{t('rerankModel')}</FormLabel>
          <FormControl>
            <ModelTreeSelect
              modelTypes={['rerank']}
              allowClear
              placeholder={t('rerankPlaceholder')}
              {...field}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

export const rerankFormSchema = {
  [RerankId]: z.string().optional(),
  top_k: z.coerce.number().optional(),
};

export function RerankFormFields() {
  const { watch } = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const rerankId = watch(RerankId);

  return (
    <>
      <RerankFormField></RerankFormField>
      {rerankId && (
        <SliderInputFormField
          name={'top_k'}
          label={t('topK')}
          max={2048}
          min={1}
          tooltip={t('topKTip')}
        ></SliderInputFormField>
      )}
    </>
  );
}

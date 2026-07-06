import { ModelTreeSelect } from '@/components/model-tree-select';
import { useTranslate } from '@/hooks/common-hooks';
import { prefixName } from '@/utils/form';
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

const DefaultRerankId = 'rerank_id';
const DefaultTopK = 'top_k';

interface RerankFormFieldProps {
  name?: string;
}

function RerankFormField({ name = DefaultRerankId }: RerankFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name={name}
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
  [DefaultRerankId]: z.string().optional(),
  top_k: z.coerce.number().optional(),
};

interface RerankFormFieldsProps {
  prefix?: string;
}

export function RerankFormFields({ prefix = '' }: RerankFormFieldsProps) {
  const { watch } = useFormContext();
  const { t } = useTranslate('knowledgeDetails');
  const rerankIdName = prefixName(prefix, DefaultRerankId);
  const topKName = prefixName(prefix, DefaultTopK);

  const rerankId = watch(rerankIdName);

  return (
    <>
      <RerankFormField name={rerankIdName}></RerankFormField>
      {rerankId && (
        <SliderInputFormField
          name={topKName}
          label={t('topK')}
          max={2048}
          min={1}
          tooltip={t('topKTip')}
        ></SliderInputFormField>
      )}
    </>
  );
}

import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';

interface SimilaritySliderFormFieldProps {
  max?: number;
}

export const DEFAULT_TOP_N_MAX = 200;

export const createTopNSchema = (max = DEFAULT_TOP_N_MAX) => ({
  top_n: z.number().max(max).optional(),
});

export const topnSchema = createTopNSchema();

export function TopNFormField({
  max = DEFAULT_TOP_N_MAX,
}: SimilaritySliderFormFieldProps) {
  const { t } = useTranslate('chat');

  return (
    <SliderInputFormField
      name={'top_n'}
      label={t('topN')}
      max={max}
      tooltip={t('topNTip')}
      layout={FormLayout.Vertical}
    ></SliderInputFormField>
  );
}

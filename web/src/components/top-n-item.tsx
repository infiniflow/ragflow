import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';

interface SimilaritySliderFormFieldProps {
  max?: number;
}

const TOP_N_MAX = 200;

export const topnSchema = {
  top_n: z.number().max(TOP_N_MAX).optional(),
};

export function TopNFormField({ max = TOP_N_MAX }: SimilaritySliderFormFieldProps) {
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

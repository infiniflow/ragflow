import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';

interface SimilaritySliderFormFieldProps {
  max?: number;
  name?: string;
}

export const topnSchema = {
  top_n: z.number().optional(),
};

export function TopNFormField({
  max = 30,
  name = 'top_n',
}: SimilaritySliderFormFieldProps) {
  const { t } = useTranslate('chat');

  return (
    <SliderInputFormField
      name={name}
      label={t('topN')}
      max={max}
      tooltip={t('topNTip')}
      layout={FormLayout.Vertical}
    ></SliderInputFormField>
  );
}

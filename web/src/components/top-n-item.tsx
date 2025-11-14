import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Slider } from 'antd';
import { z } from 'zod';
import { SliderInputFormField } from './slider-input-form-field';

type FieldType = {
  top_n?: number;
};

interface IProps {
  initialValue?: number;
  max?: number;
}

const TopNItem = ({ initialValue = 8, max = 30 }: IProps) => {
  const { t } = useTranslate('chat');

  return (
    <Form.Item<FieldType>
      label={t('topN')}
      name={'top_n'}
      initialValue={initialValue}
      tooltip={t('topNTip')}
    >
      <Slider max={max} />
    </Form.Item>
  );
};

export default TopNItem;

interface SimilaritySliderFormFieldProps {
  max?: number;
}

export const topnSchema = {
  top_n: z.number().optional(),
};

export function TopNFormField({ max = 30 }: SimilaritySliderFormFieldProps) {
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

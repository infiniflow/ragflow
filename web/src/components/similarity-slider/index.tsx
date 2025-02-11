import { useTranslate } from '@/hooks/common-hooks';
import { Form, Slider } from 'antd';
import { useFormContext } from 'react-hook-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { FormSlider } from '../ui/slider';

type FieldType = {
  similarity_threshold?: number;
  // vector_similarity_weight?: number;
};

interface IProps {
  isTooltipShown?: boolean;
  vectorSimilarityWeightName?: string;
}

const SimilaritySlider = ({
  isTooltipShown = false,
  vectorSimilarityWeightName = 'vector_similarity_weight',
}: IProps) => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <>
      <Form.Item<FieldType>
        label={t('similarityThreshold')}
        name={'similarity_threshold'}
        tooltip={isTooltipShown && t('similarityThresholdTip')}
        initialValue={0.2}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
      <Form.Item
        label={t('vectorSimilarityWeight')}
        name={vectorSimilarityWeightName}
        initialValue={1 - 0.3}
        tooltip={isTooltipShown && t('vectorSimilarityWeightTip')}
      >
        <Slider max={1} step={0.01} />
      </Form.Item>
    </>
  );
};

export default SimilaritySlider;

interface SimilaritySliderFormFieldProps {
  name?: string;
}

export function SimilaritySliderFormField({
  name = 'vector_similarity_weight',
}: SimilaritySliderFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('vectorSimilarityWeight')}</FormLabel>
          <FormControl>
            <FormSlider {...field}></FormSlider>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

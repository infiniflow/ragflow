import { useTranslate } from '@/hooks/common-hooks';
import { Form, Slider } from 'antd';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { SingleFormSlider } from '../ui/dual-range-slider';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';

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
  vectorSimilarityWeightName?: string;
  isTooltipShown?: boolean;
}

export const initialSimilarityThresholdValue = {
  similarity_threshold: 0.2,
};
export const initialKeywordsSimilarityWeightValue = {
  keywords_similarity_weight: 0.7,
};

export const similarityThresholdSchema = { similarity_threshold: z.number() };

export const keywordsSimilarityWeightSchema = {
  keywords_similarity_weight: z.number(),
};

export function SimilaritySliderFormField({
  vectorSimilarityWeightName = 'vector_similarity_weight',
  isTooltipShown,
}: SimilaritySliderFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <>
      <FormField
        control={form.control}
        name={'similarity_threshold'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={isTooltipShown && t('similarityThresholdTip')}>
              {t('similarityThreshold')}
            </FormLabel>
            <FormControl>
              <SingleFormSlider
                {...field}
                max={1}
                step={0.01}
              ></SingleFormSlider>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={vectorSimilarityWeightName}
        render={({ field }) => (
          <FormItem>
            <FormLabel
              tooltip={isTooltipShown && t('vectorSimilarityWeightTip')}
            >
              {t('vectorSimilarityWeight')}
            </FormLabel>
            <FormControl>
              <SingleFormSlider
                {...field}
                max={1}
                step={0.01}
              ></SingleFormSlider>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { SliderInputFormField } from '../slider-input-form-field';
import { SingleFormSlider } from '../ui/dual-range-slider';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { NumberInput } from '../ui/input';

interface SimilaritySliderFormFieldProps {
  similarityName?: string;
  similarityWeightName?: string;
  similarityWeightType?: 'vector' | 'keyword';
  isTooltipShown?: boolean;
  numberInputClassName?: string;
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

export const vectorSimilarityWeightSchema = {
  vector_similarity_weight: z.number(),
};

export const initialVectorSimilarityWeightValue = {
  vector_similarity_weight: 0.3,
};

export function SimilaritySliderFormField({
  similarityName = 'similarity_threshold',
  similarityWeightName = 'vector_similarity_weight',
  similarityWeightType = 'vector',
  isTooltipShown,
  numberInputClassName,
}: SimilaritySliderFormFieldProps) {
  const { t } = useTranslate('knowledgeDetails');
  const form = useFormContext();
  const isVector = similarityWeightType === 'vector';
  const normalizeWeight = (weight: number) => Number(weight.toFixed(2));
  const getVectorWeight = (weight: number) =>
    normalizeWeight(isVector ? weight : 1 - weight);
  const getFullTextWeight = (weight: number) =>
    normalizeWeight(isVector ? 1 - weight : weight);
  const getStoredWeight = (vectorWeight: number) =>
    normalizeWeight(isVector ? vectorWeight : 1 - vectorWeight);

  return (
    <>
      <SliderInputFormField
        name={similarityName}
        label={t('similarityThreshold')}
        max={1}
        step={0.01}
        layout={FormLayout.Vertical}
        tooltip={isTooltipShown && t('similarityThresholdTip')}
        numberInputClassName={numberInputClassName}
      ></SliderInputFormField>
      <FormField
        control={form.control}
        name={similarityWeightName}
        defaultValue={0}
        render={({ field }) => (
          <FormItem
          // className={cn({ 'flex items-center gap-1 space-y-0': isHorizontal })}
          >
            <FormLabel
              tooltip={
                isTooltipShown &&
                t(
                  isVector
                    ? 'vectorSimilarityWeightTip'
                    : 'keywordSimilarityWeightTip',
                )
              }
            >
              {t(
                isVector ? 'vectorSimilarityWeight' : 'keywordSimilarityWeight',
              )}
            </FormLabel>
            <div className={cn('flex items-end gap-4 justify-between')}>
              <FormControl>
                <div className="flex flex-col flex-1 gap-2">
                  <div className="flex justify-between items-center">
                    <div className="flex items-center gap-1">
                      <label className="italic text-xs text-text-secondary">
                        vector
                      </label>
                      <span className="bg-bg-card rounded-md p-1 w-10 text-center text-xs">
                        {getVectorWeight(field.value).toFixed(2)}
                      </span>
                    </div>
                    <div className="flex  items-center gap-1">
                      <label className="italic text-xs text-text-secondary">
                        full-text
                      </label>
                      <span className="bg-bg-card rounded-md p-1 w-10 text-center text-xs">
                        {getFullTextWeight(field.value).toFixed(2)}
                      </span>
                    </div>
                  </div>
                  <SingleFormSlider
                    {...field}
                    value={getVectorWeight(field.value)}
                    onChange={(value) => field.onChange(getStoredWeight(value))}
                    max={1}
                    step={0.01}
                    min={0}
                  ></SingleFormSlider>
                </div>
              </FormControl>
              <FormControl>
                <NumberInput
                  className={cn(
                    'h-6 w-10 p-0 text-center bg-bg-input border-border-button border text-text-secondary',
                    '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none',
                    numberInputClassName,
                  )}
                  max={1}
                  min={0}
                  step={0.01}
                  {...field}
                  value={getVectorWeight(field.value)}
                  onChange={(value) => field.onChange(getStoredWeight(value))}
                ></NumberInput>
              </FormControl>
            </div>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

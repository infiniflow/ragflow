'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import { RerankFormFields } from '@/components/rerank';
import {
  initialKeywordsSimilarityWeightValue,
  initialSimilarityThresholdValue,
  keywordsSimilarityWeightSchema,
  SimilaritySliderFormField,
  similarityThresholdSchema,
} from '@/components/similarity-slider';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { LoadingButton } from '@/components/ui/loading-button';
import { Textarea } from '@/components/ui/textarea';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { trim } from 'lodash';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

type TestingFormProps = Pick<
  ReturnType<typeof useTestRetrieval>,
  'loading' | 'refetch' | 'setValues'
>;

export default function TestingForm({
  loading,
  refetch,
  setValues,
}: TestingFormProps) {
  const { t } = useTranslation();

  const formSchema = z.object({
    question: z.string().min(1, {
      message: t('knowledgeDetails.testTextPlaceholder'),
    }),
    ...similarityThresholdSchema,
    ...keywordsSimilarityWeightSchema,
  });

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      ...initialSimilarityThresholdValue,
      ...initialKeywordsSimilarityWeightValue,
    },
  });

  const question = form.watch('question');

  const values = useWatch({ control: form.control });

  useEffect(() => {
    setValues(values as Required<z.infer<typeof formSchema>>);
  }, [setValues, values]);

  function onSubmit() {
    refetch();
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
        <SimilaritySliderFormField
          vectorSimilarityWeightName="keywords_similarity_weight"
          isTooltipShown
        ></SimilaritySliderFormField>
        <RerankFormFields></RerankFormFields>
        <UseKnowledgeGraphFormField name="use_kg"></UseKnowledgeGraphFormField>
        <FormField
          control={form.control}
          name="question"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('knowledgeDetails.testText')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  className="bg-colors-background-inverse-weak"
                ></Textarea>
              </FormControl>

              <FormMessage />
            </FormItem>
          )}
        />
        <LoadingButton
          variant={'tertiary'}
          size={'sm'}
          type="submit"
          className="w-full"
          disabled={!!!trim(question)}
          loading={loading}
        >
          {t('knowledgeDetails.testingLabel')}
        </LoadingButton>
      </form>
    </Form>
  );
}

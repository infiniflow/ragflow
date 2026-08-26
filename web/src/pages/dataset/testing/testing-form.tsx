'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { FormContainer } from '@/components/form-container';
import {
  RerankCandidatesCountFormField,
  rerankCandidatesCountSchema,
} from '@/components/rerank-candidates-count-item';
import {
  MetadataFilter,
  MetadataFilterSchema,
} from '@/components/metadata-filter';
import { RerankFormFields } from '@/components/rerank';
import {
  SimilaritySliderFormField,
  initialSimilarityThresholdValue,
  initialVectorSimilarityWeightValue,
  similarityThresholdSchema,
  vectorSimilarityWeightSchema,
} from '@/components/similarity-slider';
import { TopSelectFormItem } from '@/components/top-select';
import { ButtonLoading } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';

import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { ITestRetrievalRequestBody } from '@/interfaces/request/knowledge';
import { trim } from 'lodash';
import { Send } from 'lucide-react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { useOwnerTenantId } from '../contexts/knowledge-base-context';

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
  const { id } = useParams();
  const ownerTenantId = useOwnerTenantId();
  const knowledgeBaseId = id;

  const formSchema = z
    .object({
      question: z.string().min(1, {
        message: t('knowledgeDetails.testTextPlaceholder'),
      }),
      ...similarityThresholdSchema,
      ...vectorSimilarityWeightSchema,
      dataset_ids: z.array(z.string()).optional(),
      ...MetadataFilterSchema,
      size: z.number().int().min(1).max(100),
      ...rerankCandidatesCountSchema,
    })
    .refine((values) => values.rerank_candidates_count >= values.size, {
      message: t('chat.rerankCandidatesCountValidation'),
      path: ['rerank_candidates_count'],
    });

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      ...initialSimilarityThresholdValue,
      ...initialVectorSimilarityWeightValue,
      dataset_ids: [knowledgeBaseId],
      size: 10,
      rerank_candidates_count: 64,
    },
  });

  const question = form.watch('question');

  const values = useWatch({ control: form.control });

  useEffect(() => {
    setValues(values as ITestRetrievalRequestBody);
  }, [setValues, values]);

  function onSubmit() {
    refetch();
  }

  return (
    <Form {...form}>
      <form
        className="size-full flex flex-col"
        onSubmit={form.handleSubmit(onSubmit)}
      >
        <div className="px-5 h-0 flex-1">
          <FormContainer className="p-5 h-full overflow-auto">
            <SimilaritySliderFormField
              isTooltipShown={true}
            ></SimilaritySliderFormField>
            <RerankFormFields ownerTenantId={ownerTenantId}></RerankFormFields>
            <CrossLanguageFormField
              name={'cross_languages'}
            ></CrossLanguageFormField>
            <MetadataFilter prefix=""></MetadataFilter>
            <RerankCandidatesCountFormField></RerankCandidatesCountFormField>
            <TopSelectFormItem></TopSelectFormItem>
          </FormContainer>
        </div>

        <footer className="flex-0 p-5">
          <FormField
            control={form.control}
            name="question"
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <Textarea
                    {...field}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault();
                        form.handleSubmit(onSubmit)();
                      }
                    }}
                  ></Textarea>
                </FormControl>

                <FormMessage />
              </FormItem>
            )}
          />

          <div className="mt-2.5 text-end">
            <ButtonLoading
              type="submit"
              disabled={!trim(question)}
              loading={loading}
            >
              {/* {!loading && <CirclePlay />} */}
              {t('knowledgeDetails.testingLabel')}
              <Send />
            </ButtonLoading>
          </div>
        </footer>
      </form>
    </Form>
  );
}

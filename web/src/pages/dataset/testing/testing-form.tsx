'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { trim } from 'lodash';
import { useTranslation } from 'react-i18next';

export default function TestingForm() {
  const { t } = useTranslation();

  const formSchema = z.object({
    question: z.string().min(1, {
      message: t('knowledgeDetails.testTextPlaceholder'),
    }),
  });

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {},
  });

  const question = form.watch('question');

  function onSubmit(values: z.infer<typeof formSchema>) {
    console.log(values);
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
        <SimilaritySliderFormField
          vectorSimilarityWeightName="keywords_similarity_weight"
          isTooltipShown
        ></SimilaritySliderFormField>
        <RerankFormFields></RerankFormFields>
        <UseKnowledgeGraphFormField name="prompt_config.use_kg"></UseKnowledgeGraphFormField>
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
        <Button
          variant={'tertiary'}
          size={'sm'}
          type="submit"
          className="w-full"
          disabled={!!!trim(question)}
        >
          {t('knowledgeDetails.testingLabel')}
        </Button>
      </form>
    </Form>
  );
}

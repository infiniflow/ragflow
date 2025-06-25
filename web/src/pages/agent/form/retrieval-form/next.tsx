import { FormContainer } from '@/components/form-container';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialRetrievalValues } from '../../constant';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { useValues } from './use-values';

export const RetrievalPartialSchema = {
  similarity_threshold: z.coerce.number(),
  keywords_similarity_weight: z.coerce.number(),
  top_n: z.coerce.number(),
  top_k: z.coerce.number(),
  kb_ids: z.array(z.string()),
  rerank_id: z.string(),
  empty_response: z.string(),
};

export const FormSchema = z.object({
  query: z.string().optional(),
  ...RetrievalPartialSchema,
});

export function EmptyResponseField() {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name="empty_response"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('chat.emptyResponse')}</FormLabel>
          <FormControl>
            <Textarea
              placeholder={t('common.namePlaceholder')}
              {...field}
              autoComplete="off"
              rows={4}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

const RetrievalForm = ({ node }: INextOperatorForm) => {
  const outputList = useMemo(() => {
    return [
      {
        title: 'formalized_content',
        type: initialRetrievalValues.outputs.formalized_content.type,
      },
    ];
  }, []);

  const defaultValues = useValues(node);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <QueryVariable></QueryVariable>
          <KnowledgeBaseFormField></KnowledgeBaseFormField>
        </FormContainer>
        <FormContainer>
          <SimilaritySliderFormField
            vectorSimilarityWeightName="keywords_similarity_weight"
            isTooltipShown
          ></SimilaritySliderFormField>
          <TopNFormField></TopNFormField>
          <RerankFormFields></RerankFormFields>
          <EmptyResponseField></EmptyResponseField>
        </FormContainer>
        <Output list={outputList}></Output>
      </form>
    </Form>
  );
};

export default RetrievalForm;

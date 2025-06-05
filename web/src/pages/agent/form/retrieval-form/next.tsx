import { FormContainer } from '@/components/form-container';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { RerankFormFields } from '@/components/rerank';
import {
  initialKeywordsSimilarityWeightValue,
  initialSimilarityThresholdValue,
  SimilaritySliderFormField,
} from '@/components/similarity-slider';
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
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { INextOperatorForm } from '../../interface';
import { QueryVariable } from '../components/query-variable';

const FormSchema = z.object({
  query: z.string().optional(),
  similarity_threshold: z.coerce.number(),
  keywords_similarity_weight: z.coerce.number(),
  top_n: z.coerce.number(),
  top_k: z.coerce.number(),
  kb_ids: z.array(z.string()),
  rerank_id: z.string(),
  empty_response: z.string(),
});

const defaultValues = {
  query: '',
  top_n: 0.2,
  top_k: 1024,
  kb_ids: [],
  rerank_id: '',
  empty_response: '',
  ...initialSimilarityThresholdValue,
  ...initialKeywordsSimilarityWeightValue,
};

const RetrievalForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

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
        </FormContainer>
      </form>
    </Form>
  );
};

export default RetrievalForm;

import { FormContainer } from '@/components/form-container';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { DescriptionField } from '../../components/description-field';
import {
  EmptyResponseField,
  RetrievalPartialSchema,
} from '../../retrieval-form/next';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

export const FormSchema = z.object({
  ...RetrievalPartialSchema,
  description: z.string().optional(),
});

const RetrievalForm = () => {
  const defaultValues = useValues();

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <DescriptionField></DescriptionField>
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
      </form>
    </Form>
  );
};

export default RetrievalForm;

import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { FormContainer } from '@/components/form-container';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { DescriptionField } from '../../components/description-field';
import { FormWrapper } from '../../components/form-wrapper';
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
      <FormWrapper>
        <FormContainer>
          <DescriptionField></DescriptionField>
          <KnowledgeBaseFormField showVariable></KnowledgeBaseFormField>
        </FormContainer>
        <Collapse title={<div>{t('flow.advancedSettings')}</div>}>
          <FormContainer>
            <SimilaritySliderFormField
              vectorSimilarityWeightName="keywords_similarity_weight"
              isTooltipShown
            ></SimilaritySliderFormField>
            <TopNFormField></TopNFormField>
            <RerankFormFields></RerankFormFields>
            <EmptyResponseField></EmptyResponseField>
            <CrossLanguageFormField name="cross_languages"></CrossLanguageFormField>
            <UseKnowledgeGraphFormField name="use_kg"></UseKnowledgeGraphFormField>
          </FormContainer>
        </Collapse>
      </FormWrapper>
    </Form>
  );
};

export default RetrievalForm;

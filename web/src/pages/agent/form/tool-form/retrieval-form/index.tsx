import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { FormContainer } from '@/components/form-container';
import { MetadataFilter } from '@/components/metadata-filter';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { RerankCandidatesCountFormField } from '@/components/rerank-candidates-count-item';

import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import {
  useRevalidateStaleDatasetIds,
  useStaleDatasetFormSchema,
} from '@/hooks/use-stale-dataset-validation';

import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { omit } from 'lodash';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useOwnerTenantId } from '../../../context';
import { DescriptionField } from '../../components/description-field';
import { FormWrapper } from '../../components/form-wrapper';
import {
  MemoryDatasetForm,
  RetrievalPartialSchema,
  useHideKnowledgeGraphField,
} from '../../retrieval-form/next';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

export const FormSchema = z.object({
  ...RetrievalPartialSchema,
  description: z.string().optional(),
});

const RetrievalForm = () => {
  const defaultValues = omit(useValues(), 'top_k');

  const { formSchema, datasetsFetched } = useStaleDatasetFormSchema(
    FormSchema,
    defaultValues?.dataset_ids,
  );

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(formSchema),
    mode: 'onChange',
  });

  const hideKnowledgeGraphField = useHideKnowledgeGraphField(form);

  useWatchFormChange(form);

  useRevalidateStaleDatasetIds(form, datasetsFetched);

  const ownerTenantId = useOwnerTenantId();

  return (
    <Form {...form}>
      <FormWrapper>
        <DescriptionField></DescriptionField>
        <MemoryDatasetForm></MemoryDatasetForm>
        <Collapse defaultOpen title={<div>{t('flow.advancedSettings')}</div>}>
          <FormContainer>
            <SimilaritySliderFormField
              similarityWeightName="keywords_similarity_weight"
              similarityWeightType="keyword"
              isTooltipShown
            ></SimilaritySliderFormField>
            <RerankCandidatesCountFormField></RerankCandidatesCountFormField>
            <TopNFormField></TopNFormField>
            {hideKnowledgeGraphField || (
              <>
                <RerankFormFields
                  ownerTenantId={ownerTenantId}
                ></RerankFormFields>
                <MetadataFilter canReference></MetadataFilter>
              </>
            )}

            {hideKnowledgeGraphField || (
              <>
                <CrossLanguageFormField name="cross_languages"></CrossLanguageFormField>
              </>
            )}
          </FormContainer>
        </Collapse>
      </FormWrapper>
    </Form>
  );
};

export default RetrievalForm;

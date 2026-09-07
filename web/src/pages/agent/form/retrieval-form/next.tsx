import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { MemoriesFormField } from '@/components/memories-form-field';
import {
  MetadataFilter,
  MetadataFilterSchema,
} from '@/components/metadata-filter';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import {
  RerankCandidatesCountFormField,
  rerankCandidatesCountSchema,
} from '@/components/rerank-candidates-count-item';

import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { Radio } from '@/components/ui/radio';
import {
  useRevalidateStaleDatasetIds,
  useStaleDatasetFormSchema,
} from '@/hooks/use-stale-dataset-validation';

import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { memo, useMemo } from 'react';
import {
  UseFormReturn,
  useForm,
  useFormContext,
  useWatch,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { RetrievalFrom, initialRetrievalValues } from '../../constant';
import { useOwnerTenantId } from '../../context';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { UserIdFormField } from '../components/user-id-form-field';
import { useValues } from './use-values';

export const RetrievalPartialSchema = {
  similarity_threshold: z.coerce.number(),
  keywords_similarity_weight: z.coerce.number().min(0).max(1),
  top_n: z.coerce.number(),
  ...rerankCandidatesCountSchema,
  dataset_ids: z.array(z.string()).optional(),
  rerank_id: z.string(),
  cross_languages: z.array(z.string()),
  ...MetadataFilterSchema,
  memory_ids: z.array(z.string()).optional(),
  retrieval_from: z.string(),
  user_id: z.string().optional(),
};

export const FormSchema = z
  .object({
    query: z.string().optional(),
    ...RetrievalPartialSchema,
  })
  .superRefine((data, ctx) => {
    // A Retrieval node sourcing from datasets must name at least one dataset,
    // and one sourcing from memories must name at least one memory. The
    // backend otherwise rejects the run with a `dataset_ids`/`memory_ids is
    // required` error that only surfaces at runtime.
    if (
      data.retrieval_from === RetrievalFrom.Dataset &&
      (data.dataset_ids ?? []).length === 0
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dataset_ids'],
        message: t('flow.retrievalDatasetRequired'),
      });
    }
    if (
      data.retrieval_from === RetrievalFrom.Memory &&
      (data.memory_ids ?? []).length === 0
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['memory_ids'],
        message: t('flow.retrievalMemoryRequired'),
      });
    }
  });

export type RetrievalFormSchemaType = z.infer<typeof FormSchema>;

export function MemoryDatasetForm() {
  const { t } = useTranslation();
  const form = useFormContext();
  const retrievalFrom = useWatch({
    control: form.control,
    name: 'retrieval_from',
  });

  return (
    <>
      <RAGFlowFormItem name="retrieval_from" label={t('flow.retrievalFrom')}>
        <Radio.Group>
          <Radio value={RetrievalFrom.Dataset}>
            {t('knowledgeDetails.dataset')}
          </Radio>
          <Radio value={RetrievalFrom.Memory}>{t('header.memories')}</Radio>
        </Radio.Group>
      </RAGFlowFormItem>
      {retrievalFrom === RetrievalFrom.Memory ? (
        <>
          <MemoriesFormField
            label={t('header.memories')}
            required
          ></MemoriesFormField>
          <UserIdFormField></UserIdFormField>
        </>
      ) : (
        <KnowledgeBaseFormField showVariable required></KnowledgeBaseFormField>
      )}
    </>
  );
}

export function useHideKnowledgeGraphField(form: UseFormReturn<any>) {
  const retrievalFrom = useWatch({
    control: form.control,
    name: 'retrieval_from',
  });

  return retrievalFrom === RetrievalFrom.Memory;
}

function RetrievalForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();

  const outputList = useMemo(() => {
    return [
      {
        title: 'formalized_content',
        type: initialRetrievalValues.outputs.formalized_content.type,
      },
      {
        title: 'json',
        type: initialRetrievalValues.outputs.json.type,
      },
    ];
  }, []);

  const defaultValues = useValues(node);

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

  useWatchFormChange(node?.id, form);

  useRevalidateStaleDatasetIds(form, datasetsFetched);

  return (
    <Form {...form}>
      <FormWrapper>
        <RAGFlowFormItem name="query" label={t('flow.query')}>
          <PromptEditor></PromptEditor>
        </RAGFlowFormItem>
        <MemoryDatasetForm></MemoryDatasetForm>
        <Collapse defaultOpen title={<div>{t('flow.advancedSettings')}</div>}>
          <section className="space-y-5">
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
          </section>
        </Collapse>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(RetrievalForm);

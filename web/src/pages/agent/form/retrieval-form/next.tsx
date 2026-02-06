import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { FormContainer } from '@/components/form-container';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { MemoriesFormField } from '@/components/memories-form-field';
import {
  MetadataFilter,
  MetadataFilterSchema,
} from '@/components/metadata-filter';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { TOCEnhanceFormField } from '@/components/toc-enhance-form-field';
import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Radio } from '@/components/ui/radio';
import { Textarea } from '@/components/ui/textarea';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { zodResolver } from '@hookform/resolvers/zod';
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
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { useValues } from './use-values';

export const RetrievalPartialSchema = {
  similarity_threshold: z.coerce.number(),
  keywords_similarity_weight: z.coerce.number(),
  top_n: z.coerce.number(),
  top_k: z.coerce.number(),
  kb_ids: z.array(z.string()),
  rerank_id: z.string(),
  empty_response: z.string(),
  cross_languages: z.array(z.string()),
  use_kg: z.boolean(),
  toc_enhance: z.boolean(),
  ...MetadataFilterSchema,
  memory_ids: z.array(z.string()).optional(),
  retrieval_from: z.string(),
};

export const FormSchema = z.object({
  query: z.string().optional(),
  ...RetrievalPartialSchema,
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
        <MemoriesFormField label={t('header.memories')}></MemoriesFormField>
      ) : (
        <KnowledgeBaseFormField showVariable></KnowledgeBaseFormField>
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

export function EmptyResponseField() {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name="empty_response"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.emptyResponseTip')}>
            {t('chat.emptyResponse')}
          </FormLabel>
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

function RetrievalForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

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

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const hideKnowledgeGraphField = useHideKnowledgeGraphField(form);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <RAGFlowFormItem name="query" label={t('flow.query')}>
          <PromptEditor></PromptEditor>
        </RAGFlowFormItem>
        <MemoryDatasetForm></MemoryDatasetForm>
        <Collapse title={<div>{t('flow.advancedSettings')}</div>}>
          <FormContainer>
            <SimilaritySliderFormField
              vectorSimilarityWeightName="keywords_similarity_weight"
              isTooltipShown
            ></SimilaritySliderFormField>
            <TopNFormField></TopNFormField>
            {hideKnowledgeGraphField || (
              <>
                <RerankFormFields></RerankFormFields>
                <MetadataFilter canReference></MetadataFilter>
              </>
            )}
            <EmptyResponseField></EmptyResponseField>
            {hideKnowledgeGraphField || (
              <>
                <CrossLanguageFormField name="cross_languages"></CrossLanguageFormField>
                <UseKnowledgeGraphFormField name="use_kg"></UseKnowledgeGraphFormField>
                <TOCEnhanceFormField name="toc_enhance"></TOCEnhanceFormField>
              </>
            )}
          </FormContainer>
        </Collapse>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(RetrievalForm);

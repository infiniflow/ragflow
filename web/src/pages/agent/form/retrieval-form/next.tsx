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
import { useTranslation } from 'react-i18next';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

const RetrievalForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslation();
  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <SimilaritySliderFormField
          vectorSimilarityWeightName="keywords_similarity_weight"
          isTooltipShown
        ></SimilaritySliderFormField>
        <TopNFormField></TopNFormField>
        <RerankFormFields></RerankFormFields>
        <KnowledgeBaseFormField></KnowledgeBaseFormField>
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
      </form>
    </Form>
  );
};

export default RetrievalForm;

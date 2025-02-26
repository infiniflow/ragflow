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
import { useTranslate } from '@/hooks/common-hooks';
import { INextOperatorForm } from '../../interface';
import { DynamicVariableForm } from '../components/next-dynamic-input-variable';

const RetrievalForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');
  return (
    <Form {...form}>
      <DynamicVariableForm></DynamicVariableForm>
      <SimilaritySliderFormField name="keywords_similarity_weight"></SimilaritySliderFormField>
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
    </Form>
  );
};

export default RetrievalForm;

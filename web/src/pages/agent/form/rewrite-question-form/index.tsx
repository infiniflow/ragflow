import { NextLLMSelect } from '@/components/llm-select';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { useTranslation } from 'react-i18next';
import { GoogleLanguageOptions } from '../../constant';
import { INextOperatorForm } from '../../interface';

const RewriteQuestionForm = ({ form }: INextOperatorForm) => {
  const { t } = useTranslation();

  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormField
          control={form.control}
          name="llm_id"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.modelTip')}>
                {t('chat.model')}
              </FormLabel>
              <FormControl>
                <NextLLMSelect {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="language"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.languageTip')}>
                {t('chat.language')}
              </FormLabel>
              <FormControl>
                <RAGFlowSelect
                  options={GoogleLanguageOptions}
                  allowClear={true}
                  {...field}
                ></RAGFlowSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
      </form>
    </Form>
  );
};

export default RewriteQuestionForm;

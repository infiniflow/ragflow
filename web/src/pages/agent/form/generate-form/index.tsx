import { NextLLMSelect } from '@/components/llm-select';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { PromptEditor } from '@/components/prompt-editor';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { useTranslation } from 'react-i18next';
import { INextOperatorForm } from '../../interface';

const GenerateForm = ({ form }: INextOperatorForm) => {
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
          name="prompt"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.promptTip')}>
                {t('flow.systemPrompt')}
              </FormLabel>
              <FormControl>
                <PromptEditor {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="cite"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.citeTip')}>
                {t('flow.cite')}
              </FormLabel>
              <FormControl>
                <Switch {...field} />
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

export default GenerateForm;

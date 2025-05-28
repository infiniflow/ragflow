import { LargeModelFormField } from '@/components/large-model-form-field';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useTranslation } from 'react-i18next';
import { INextOperatorForm } from '../../interface';
import DynamicCategorize from './dynamic-categorize';

const CategorizeForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslation();

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-5 "
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormField
          control={form.control}
          name="input"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.modelTip')}>
                {t('chat.input')}
              </FormLabel>
              <FormControl>
                <SelectWithSearch {...field}></SelectWithSearch>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <LargeModelFormField></LargeModelFormField>
        <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
        <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
      </form>
    </Form>
  );
};

export default CategorizeForm;

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { NextLLMSelect } from './llm-select';

export function LargeModelFormField() {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name="llm_id"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.modelTip')}>{t('chat.model')}</FormLabel>
          <FormControl>
            <NextLLMSelect {...field} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

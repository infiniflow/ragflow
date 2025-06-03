import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function QueryVariable() {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name="query"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.modelTip')}>{t('flow.query')}</FormLabel>
          <FormControl>
            <SelectWithSearch {...field}></SelectWithSearch>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

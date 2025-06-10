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
import { useBuildQueryVariableOptions } from '../../hooks/use-get-begin-query';

export function QueryVariable() {
  const { t } = useTranslation();
  const form = useFormContext();

  const nextOptions = useBuildQueryVariableOptions();

  return (
    <FormField
      control={form.control}
      name="query"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.modelTip')}>{t('flow.query')}</FormLabel>
          <FormControl>
            <SelectWithSearch
              options={nextOptions}
              {...field}
            ></SelectWithSearch>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

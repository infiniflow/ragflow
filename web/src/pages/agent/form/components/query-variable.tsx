import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { VariableType } from '../../constant';
import { useBuildQueryVariableOptions } from '../../hooks/use-get-begin-query';

type QueryVariableProps = { name?: string; type?: VariableType };

export function QueryVariable({ name = 'query', type }: QueryVariableProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const nextOptions = useBuildQueryVariableOptions();

  const finalOptions = useMemo(() => {
    return type
      ? nextOptions.map((x) => {
          return { ...x, options: x.options.filter((y) => y.type === type) };
        })
      : nextOptions;
  }, [nextOptions, type]);

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chat.modelTip')}>{t('flow.query')}</FormLabel>
          <FormControl>
            <SelectWithSearch
              options={finalOptions}
              {...field}
            ></SelectWithSearch>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

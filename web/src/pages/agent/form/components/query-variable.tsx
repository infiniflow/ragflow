import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { toLower } from 'lodash';
import { ReactNode, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { VariableType } from '../../constant';
import { useBuildQueryVariableOptions } from '../../hooks/use-get-begin-query';

type QueryVariableProps = {
  name?: string;
  type?: VariableType;
  label?: ReactNode;
};

export function QueryVariable({
  name = 'query',
  type,
  label,
}: QueryVariableProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const nextOptions = useBuildQueryVariableOptions();

  const finalOptions = useMemo(() => {
    return type
      ? nextOptions.map((x) => {
          return {
            ...x,
            options: x.options.filter((y) => toLower(y.type).includes(type)),
          };
        })
      : nextOptions;
  }, [nextOptions, type]);

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          {label || (
            <FormLabel tooltip={t('flow.queryTip')}>
              {t('flow.query')}
            </FormLabel>
          )}
          <FormControl>
            <SelectWithSearch
              options={finalOptions}
              {...field}
              allowClear
            ></SelectWithSearch>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

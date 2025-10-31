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
import { GroupedSelectWithSecondaryMenu } from './select-with-secondary-menu';

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
            <GroupedSelectWithSecondaryMenu
              options={finalOptions}
              {...field}
              // allowClear
              type={type}
            ></GroupedSelectWithSecondaryMenu>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

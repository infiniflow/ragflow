import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { isEmpty, toLower } from 'lodash';
import { ReactNode, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { JsonSchemaDataType } from '../../constant';
import { useBuildQueryVariableOptions } from '../../hooks/use-get-begin-query';
import { GroupedSelectWithSecondaryMenu } from './select-with-secondary-menu';

type QueryVariableProps = {
  name?: string;
  types?: JsonSchemaDataType[];
  label?: ReactNode;
  hideLabel?: boolean;
  className?: string;
};

export function QueryVariable({
  name = 'query',
  types = [],
  label,
  hideLabel = false,
  className,
}: QueryVariableProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const nextOptions = useBuildQueryVariableOptions();

  const finalOptions = useMemo(() => {
    return !isEmpty(types)
      ? nextOptions.map((x) => {
          return {
            ...x,
            options: x.options.filter((y) =>
              types?.some((x) => toLower(y.type).includes(x)),
            ),
          };
        })
      : nextOptions;
  }, [nextOptions, types]);

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem className={className}>
          {hideLabel || label || (
            <FormLabel tooltip={t('flow.queryTip')}>
              {t('flow.query')}
            </FormLabel>
          )}
          <FormControl>
            <GroupedSelectWithSecondaryMenu
              options={finalOptions}
              {...field}
              // allowClear
              types={types}
            ></GroupedSelectWithSecondaryMenu>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { useContext, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { AgentFormContext } from '../../context';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';

export function QueryVariable() {
  const { t } = useTranslation();
  const form = useFormContext();
  const { data } = useFetchAgent();

  const node = useContext(AgentFormContext);
  const options = useBuildComponentIdSelectOptions(node?.id, node?.parentId);

  const nextOptions = useMemo(() => {
    const globalOptions = Object.keys(data?.dsl?.globals ?? {}).map((x) => ({
      label: x,
      value: x,
    }));
    return [
      { ...options[0], options: [...options[0]?.options, ...globalOptions] },
      ...options.slice(1),
    ];
  }, [data.dsl.globals, options]);

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

import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { WenCaiQueryTypeOptions } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

const WenCaiForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslation();

  const wenCaiQueryTypeOptions = useMemo(() => {
    return WenCaiQueryTypeOptions.map((x) => ({
      value: x,
      label: t(`flow.wenCaiQueryTypeOptions.${x}`),
    }));
  }, [t]);

  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <TopNFormField max={99}></TopNFormField>
        <FormField
          control={form.control}
          name="query_type"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.queryType')}</FormLabel>
              <FormControl>
                <RAGFlowSelect {...field} options={wenCaiQueryTypeOptions} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
};

export default WenCaiForm;

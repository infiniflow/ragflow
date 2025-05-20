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
import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import { Channel } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

const DuckDuckGoForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return Object.values(Channel).map((x) => ({ value: x, label: t(x) }));
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
        <TopNFormField></TopNFormField>
        <FormField
          control={form.control}
          name="channel"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('channelTip')}>{t('channel')}</FormLabel>
              <FormControl>
                <RAGFlowSelect {...field} options={options} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
};

export default DuckDuckGoForm;

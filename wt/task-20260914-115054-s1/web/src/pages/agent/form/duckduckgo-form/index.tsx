import { FormContainer } from '@/components/form-container';
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
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { Channel, initialDuckValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const DuckDuckGoFormPartialSchema = {
  top_n: z.string(),
  channel: z.string(),
};

const FormSchema = z.object({
  query: z.string(),
  ...DuckDuckGoFormPartialSchema,
});

export function DuckDuckGoWidgets() {
  const { t } = useTranslate('flow');
  const form = useFormContext();

  const options = useMemo(() => {
    return Object.values(Channel).map((x) => ({ value: x, label: t(x) }));
  }, [t]);

  return (
    <>
      <TopNFormField></TopNFormField>
      <FormField
        control={form.control}
        name={'channel'}
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
    </>
  );
}

const outputList = buildOutputList(initialDuckValues.outputs);

function DuckDuckGoForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialDuckValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <DuckDuckGoWidgets></DuckDuckGoWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(DuckDuckGoForm);

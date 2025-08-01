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
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialWenCaiValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { WenCaiQueryTypeOptions } from '../../options';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const WenCaiPartialSchema = {
  top_n: z.number(),
  query_type: z.string(),
};

export const FormSchema = z.object({
  ...WenCaiPartialSchema,
  query: z.string(),
});

export function WenCaiFormWidgets() {
  const { t } = useTranslation();
  const form = useFormContext();

  const wenCaiQueryTypeOptions = useMemo(() => {
    return WenCaiQueryTypeOptions.map((x) => ({
      value: x,
      label: t(`flow.wenCaiQueryTypeOptions.${x}`),
    }));
  }, [t]);

  return (
    <>
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
    </>
  );
}

const outputList = buildOutputList(initialWenCaiValues.outputs);

function WenCaiForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialWenCaiValues, node);

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
        </FormContainer>
        <FormContainer>
          <WenCaiFormWidgets></WenCaiFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(WenCaiForm);

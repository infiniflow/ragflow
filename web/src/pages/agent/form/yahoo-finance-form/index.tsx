import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { ReactNode } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialYahooFinanceValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const YahooFinanceFormPartialSchema = {
  info: z.boolean(),
  history: z.boolean(),
  financials: z.boolean(),
  balance_sheet: z.boolean(),
  cash_flow_statement: z.boolean(),
  news: z.boolean(),
};

const FormSchema = z.object({
  stock_code: z.string(),
  ...YahooFinanceFormPartialSchema,
});

interface SwitchFormFieldProps {
  name: string;
  label: ReactNode;
}
function SwitchFormField({ name, label }: SwitchFormFieldProps) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Switch
              checked={field.value}
              onCheckedChange={field.onChange}
            ></Switch>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

export function YahooFinanceFormWidgets() {
  const { t } = useTranslate('flow');
  return (
    <>
      <SwitchFormField name="info" label={t('info')}></SwitchFormField>
      <SwitchFormField name="history" label={t('history')}></SwitchFormField>
      <SwitchFormField
        name="financials"
        label={t('financials')}
      ></SwitchFormField>
      <SwitchFormField
        name="balance_sheet"
        label={t('balanceSheet')}
      ></SwitchFormField>

      <SwitchFormField
        name="cash_flow_statement"
        label={t('cashFlowStatement')}
      ></SwitchFormField>

      <SwitchFormField name="news" label={t('news')}></SwitchFormField>
    </>
  );
}

const outputList = buildOutputList(initialYahooFinanceValues.outputs);

const YahooFinanceForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');

  const defaultValues = useFormValues(initialYahooFinanceValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable
            name="stock_code"
            label={t('stockCode')}
          ></QueryVariable>
        </FormContainer>
        <FormContainer>
          <YahooFinanceFormWidgets></YahooFinanceFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default YahooFinanceForm;

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
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialBGPTValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const BGPTFormPartialSchema = {
  top_n: z.number(),
  api_key: z.string().optional(),
  days_back: z.union([z.number(), z.string()]).optional(),
};

export const FormSchema = z.object({
  ...BGPTFormPartialSchema,
  query: z.string(),
});

export function BGPTFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  return (
    <>
      <TopNFormField></TopNFormField>
      <FormField
        control={form.control}
        name="api_key"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('bgptApiKey')}</FormLabel>
            <FormControl>
              <Input {...field} type="password" placeholder={t('bgptApiKeyTip')} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="days_back"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('bgptDaysBack')}</FormLabel>
            <FormControl>
              <Input {...field} placeholder={t('bgptDaysBackTip')} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

const outputList = buildOutputList(initialBGPTValues.outputs);

function BGPTForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialBGPTValues, node);

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
          <BGPTFormWidgets></BGPTFormWidgets>
          <Output list={outputList}></Output>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(BGPTForm);

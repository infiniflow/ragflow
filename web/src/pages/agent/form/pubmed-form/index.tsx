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
import { initialPubMedValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const PubMedFormPartialSchema = {
  top_n: z.number(),
  email: z.string().email(),
};

export const FormSchema = z.object({
  ...PubMedFormPartialSchema,
  query: z.string(),
});

export function PubMedFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  return (
    <>
      <TopNFormField></TopNFormField>
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('emailTip')}>{t('email')}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

const outputList = buildOutputList(initialPubMedValues.outputs);

function PubMedForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialPubMedValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
        </FormContainer>
        <FormContainer>
          <PubMedFormWidgets></PubMedFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(PubMedForm);

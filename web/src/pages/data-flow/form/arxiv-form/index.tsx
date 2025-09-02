import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialArXivValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

export const ArXivFormPartialSchema = {
  top_n: z.number(),
  sort_by: z.string(),
};

export const FormSchema = z.object({
  ...ArXivFormPartialSchema,
  query: z.string(),
});

export function ArXivFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['submittedDate', 'lastUpdatedDate', 'relevance'].map((x) => ({
      value: x,
      label: t(x),
    }));
  }, [t]);

  return (
    <>
      <TopNFormField></TopNFormField>
      <FormField
        control={form.control}
        name={`sort_by`}
        render={({ field }) => (
          <FormItem className="flex-1">
            <FormLabel>{t('sortBy')}</FormLabel>
            <FormControl>
              <SelectWithSearch {...field} options={options}></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

const outputList = buildOutputList(initialArXivValues.outputs);

function ArXivForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialArXivValues, node);

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
          <ArXivFormWidgets></ArXivFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(ArXivForm);

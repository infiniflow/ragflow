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
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialBingValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { BingCountryOptions, BingLanguageOptions } from '../../options';
import { FormWrapper } from '../components/form-wrapper';
import { QueryVariable } from '../components/query-variable';

export const BingFormSchema = {
  channel: z.string(),
  api_key: z.string(),
  country: z.string(),
  language: z.string(),
  top_n: z.number(),
};

export const FormSchema = z.object({
  query: z.string().optional(),
  ...BingFormSchema,
});

export function BingFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['Webpages', 'News'].map((x) => ({ label: x, value: x }));
  }, []);

  return (
    <>
      <TopNFormField></TopNFormField>
      <FormField
        control={form.control}
        name="channel"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('channel')}</FormLabel>
            <FormControl>
              <SelectWithSearch {...field} options={options}></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="api_key"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('apiKey')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="country"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('country')}</FormLabel>
            <FormControl>
              <SelectWithSearch
                {...field}
                options={BingCountryOptions}
              ></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="language"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('language')}</FormLabel>
            <FormControl>
              <SelectWithSearch
                {...field}
                options={BingLanguageOptions}
              ></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

function BingForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialBingValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable></QueryVariable>
        <BingFormWidgets></BingFormWidgets>
      </FormWrapper>
    </Form>
  );
}

export default memo(BingForm);

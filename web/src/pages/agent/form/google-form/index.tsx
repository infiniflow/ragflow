import { FormContainer } from '@/components/form-container';
import NumberInput from '@/components/originui/number-input';
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
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialGoogleValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { INextOperatorForm } from '../../interface';
import { GoogleCountryOptions, GoogleLanguageOptions } from '../../options';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const outputList = buildOutputList(initialGoogleValues.outputs);

export const FormSchema = z.object({
  top_n: z.number(),
  api_key: z.string(),
  country: z.string(),
  language: z.string(),
  q: z.string(),
  start: z.number(),
  num: z.number(),
});

const GoogleForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');
  const defaultValues = useFormValues(initialGoogleValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable name="q"></QueryVariable>
        </FormContainer>
        <FormContainer>
          <ApiKeyField placeholder="YOUR_API_KEY (obtained from https://serpapi.com/manage-api-key)"></ApiKeyField>
          <FormField
            control={form.control}
            name={`start`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('start')}</FormLabel>
                <FormControl>
                  <NumberInput {...field} className="w-full"></NumberInput>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name={`num`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('num')}</FormLabel>
                <FormControl>
                  <NumberInput {...field} className="w-full"></NumberInput>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <TopNFormField></TopNFormField>
          <FormField
            control={form.control}
            name={`country`}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{t('country')}</FormLabel>
                <FormControl>
                  <SelectWithSearch
                    {...field}
                    options={GoogleCountryOptions}
                  ></SelectWithSearch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name={`language`}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{t('language')}</FormLabel>
                <FormControl>
                  <SelectWithSearch
                    {...field}
                    options={GoogleLanguageOptions}
                  ></SelectWithSearch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default GoogleForm;

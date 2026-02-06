import { FormContainer } from '@/components/form-container';
import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
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
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialGoogleValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { GoogleCountryOptions, GoogleLanguageOptions } from '../../options';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const outputList = buildOutputList(initialGoogleValues.outputs);

export const GoogleFormPartialSchema = {
  api_key: z.string(),
  country: z.string(),
  language: z.string(),
};

export const FormSchema = z.object({
  ...GoogleFormPartialSchema,
  q: z.string(),
  start: z.number(),
  num: z.number(),
});

export function GoogleFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  return (
    <>
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
    </>
  );
}

const GoogleForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');
  const defaultValues = useFormValues(initialGoogleValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable name="q"></QueryVariable>
        </FormContainer>
        <FormContainer>
          <ApiKeyField placeholder={t('apiKeyPlaceholder')}></ApiKeyField>
          <FormField
            control={form.control}
            name={`start`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flowStart')}</FormLabel>
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
                <FormLabel>{t('flowNum')}</FormLabel>
                <FormControl>
                  <NumberInput {...field} className="w-full"></NumberInput>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <GoogleFormWidgets></GoogleFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default GoogleForm;

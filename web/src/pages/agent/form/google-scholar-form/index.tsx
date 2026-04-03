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
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { DatePicker, DatePickerProps } from 'antd';
import dayjs from 'dayjs';
import { memo, useCallback, useMemo } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialGoogleScholarValues } from '../../constant';
import { useBuildSortOptions } from '../../form-hooks';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

// TODO: To be replaced
const YearPicker = ({
  onChange,
  value,
}: {
  onChange?: (val: number | undefined) => void;
  value?: number | undefined;
}) => {
  const handleChange: DatePickerProps['onChange'] = useCallback(
    (val: any) => {
      const nextVal = val?.format('YYYY');
      onChange?.(nextVal ? Number(nextVal) : undefined);
    },
    [onChange],
  );
  // The year needs to be converted into a number and saved to the backend
  const nextValue = useMemo(() => {
    if (value) {
      return dayjs(value.toString());
    }
    return undefined;
  }, [value]);

  return <DatePicker picker="year" onChange={handleChange} value={nextValue} />;
};

export function GoogleScholarFormWidgets() {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  const options = useBuildSortOptions();

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
      <FormField
        control={form.control}
        name={`year_low`}
        render={({ field }) => (
          <FormItem className="flex-1">
            <FormLabel>{t('yearLow')}</FormLabel>
            <FormControl>
              <YearPicker {...field}></YearPicker>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={`year_high`}
        render={({ field }) => (
          <FormItem className="flex-1">
            <FormLabel>{t('yearHigh')}</FormLabel>
            <FormControl>
              <YearPicker {...field}></YearPicker>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={`patents`}
        render={({ field }) => (
          <FormItem className="flex-1">
            <FormLabel>{t('patents')}</FormLabel>
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
    </>
  );
}

export const GoogleScholarFormPartialSchema = {
  top_n: z.number(),
  sort_by: z.string(),
  year_low: z.number(),
  year_high: z.number(),
  patents: z.boolean(),
};

export const FormSchema = z.object({
  ...GoogleScholarFormPartialSchema,
  query: z.string(),
});

const outputList = buildOutputList(initialGoogleScholarValues.outputs);

function GoogleScholarForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialGoogleScholarValues, node);

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
          <GoogleScholarFormWidgets></GoogleScholarFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(GoogleScholarForm);

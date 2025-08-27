import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { ButtonLoading } from '@/components/ui/button';
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
import { initialExeSqlValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { ExeSQLOptions } from '../../options';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { FormSchema, useSubmitForm } from './use-submit-form';

const outputList = buildOutputList(initialExeSqlValues.outputs);

export function ExeSQLFormWidgets({ loading }: { loading: boolean }) {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  return (
    <>
      <FormField
        control={form.control}
        name="db_type"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('dbType')}</FormLabel>
            <FormControl>
              <SelectWithSearch
                {...field}
                options={ExeSQLOptions}
              ></SelectWithSearch>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="database"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('database')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="username"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('username')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="host"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('host')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="port"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('port')}</FormLabel>
            <FormControl>
              <NumberInput {...field} className="w-full"></NumberInput>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="password"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('password')}</FormLabel>
            <FormControl>
              <Input {...field} type="password"></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="max_records"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('maxRecords')}</FormLabel>
            <FormControl>
              <NumberInput {...field} className="w-full"></NumberInput>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <div className="flex justify-end">
        <ButtonLoading loading={loading} type="submit">
          {t('test')}
        </ButtonLoading>
      </div>
    </>
  );
}

function ExeSQLForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialExeSqlValues, node);

  const { onSubmit, loading } = useSubmitForm();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper onSubmit={form.handleSubmit(onSubmit)}>
        <QueryVariable name="sql"></QueryVariable>
        <ExeSQLFormWidgets loading={loading}></ExeSQLFormWidgets>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(ExeSQLForm);

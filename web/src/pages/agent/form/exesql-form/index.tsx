import { LargeModelFormField } from '@/components/large-model-form-field';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { TopNFormField } from '@/components/top-n-item';
import { ButtonLoading } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input, NumberInput } from '@/components/ui/input';
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
import { FormWrapper } from '../components/form-wrapper';
import { QueryVariable } from '../components/query-variable';
import { FormSchema, useSubmitForm } from './use-submit-form';

export function ExeSQLFormWidgets({ loading }: { loading: boolean }) {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  return (
    <>
      <LargeModelFormField></LargeModelFormField>
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
              <NumberInput {...field}></NumberInput>
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
        name="loop"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('loopTip')}>{t('loop')}</FormLabel>
            <FormControl>
              <NumberInput {...field}></NumberInput>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <TopNFormField max={1000}></TopNFormField>
      <div className="flex justify-end">
        <ButtonLoading loading={loading} type="submit">
          Test
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
        <QueryVariable></QueryVariable>
        <ExeSQLFormWidgets loading={loading}></ExeSQLFormWidgets>
      </FormWrapper>
    </Form>
  );
}

export default memo(ExeSQLForm);

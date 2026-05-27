import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
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
import { memo, useRef, useState } from 'react';
import { useForm, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialExeSqlValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { ExeSQLOptions } from '../../options';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { FormSchema, useSubmitForm } from './use-submit-form';

const outputList = buildOutputList(initialExeSqlValues.outputs);

export function ExeSQLFormWidgets({ loading }: { loading: boolean }) {
  const form = useFormContext();
  const { t } = useTranslate('flow');
  const dbType = useWatch({ control: form.control, name: 'db_type' });
  const serviceAccountJson = useWatch({
    control: form.control,
    name: 'google_application_credentials_json',
  });
  const googleApplicationCredentialsSource = useWatch({
    control: form.control,
    name: 'google_application_credentials_source',
  });
  const serviceAccountInputRef = useRef<HTMLInputElement>(null);
  const [serviceAccountFileName, setServiceAccountFileName] = useState('');
  const hasVerifiedServiceAccount =
    typeof serviceAccountJson === 'string' &&
    serviceAccountJson.trim().length > 0;

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
      {dbType === 'BigQuery' ? (
        <>
          <FormField
            control={form.control}
            name="google_application_credentials_source"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Credentials source</FormLabel>
                <FormControl>
                  <SelectWithSearch
                    {...field}
                    options={[
                      {
                        label: 'Application Default Credentials',
                        value: 'adc',
                      },
                      { label: 'Json', value: 'json' },
                    ]}
                  ></SelectWithSearch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {googleApplicationCredentialsSource !== 'adc' ? (
            <FormField
              control={form.control}
              name="google_application_credentials_json"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Service account JSON</FormLabel>
                  {hasVerifiedServiceAccount ? (
                    <div className="mb-2">
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-emerald-700">
                        Verified
                      </span>
                    </div>
                  ) : null}
                  <FormControl>
                    <div className="space-y-2">
                      <Input
                        value={serviceAccountFileName || ''}
                        readOnly
                        placeholder="No file selected"
                      />
                      <ButtonLoading
                        type="button"
                        className="w-full"
                        onClick={() => serviceAccountInputRef.current?.click()}
                      >
                        Upload JSON
                      </ButtonLoading>
                      <input
                        ref={serviceAccountInputRef}
                        className="hidden"
                        type="file"
                        accept=".json,application/json"
                        onChange={async (e) => {
                          const file = e.target.files?.[0];
                          if (!file) {
                            return;
                          }
                          const content = await file.text();
                          field.onChange(content);
                          setServiceAccountFileName(file.name);
                        }}
                      />
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}
        </>
      ) : (
        <>
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
        </>
      )}

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
  const { t } = useTranslation();

  const { onSubmit, loading } = useSubmitForm();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper onSubmit={form.handleSubmit(onSubmit)}>
        <RAGFlowFormItem
          name="sql"
          label={t('flow.sqlStatement')}
          tooltip={t('flow.sqlStatementTip')}
        >
          <PromptEditor></PromptEditor>
        </RAGFlowFormItem>
        <ExeSQLFormWidgets loading={loading}></ExeSQLFormWidgets>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(ExeSQLForm);

import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
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
import {
  FormSchema,
  MaxMaxRecords,
  MaxPort,
  MinMaxRecords,
  MinPort,
  useSubmitForm,
} from './use-submit-form';

const outputList = buildOutputList(initialExeSqlValues.outputs);

export function ExeSQLFormWidgets({ loading }: { loading: boolean }) {
  const form = useFormContext();
  const { t } = useTranslation();
  // Password is only required for password-authenticated databases; trino
  // connects without one (mirrors the superRefine in FormSchema).
  const dbType = useWatch({ control: form.control, name: 'db_type' });

  return (
    <>
      <RAGFlowFormItem name="db_type" label={t('flow.dbType')} required>
        <SelectWithSearch options={ExeSQLOptions}></SelectWithSearch>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="database" label={t('flow.database')} required>
        <Input></Input>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="username" label={t('flow.username')} required>
        <Input></Input>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="host" label={t('flow.host')} required>
        <Input></Input>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="port" label={t('flow.port')} required>
        <NumberInput
          min={MinPort}
          max={MaxPort}
          integer
          className="w-full"
        ></NumberInput>
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name="password"
        label={t('flow.password')}
        required={dbType !== 'trino'}
      >
        <Input type="password"></Input>
      </RAGFlowFormItem>
      <RAGFlowFormItem name="max_records" label={t('flow.maxRecords')} required>
        <NumberInput
          min={MinMaxRecords}
          max={MaxMaxRecords}
          integer
          className="w-full"
        ></NumberInput>
      </RAGFlowFormItem>

      <div className="flex justify-end">
        <ButtonLoading loading={loading} type="submit">
          {t('flow.test')}
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

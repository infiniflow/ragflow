import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { memo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  JsonSchemaDataType,
  Operations,
  initialDataOperationsValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output, OutputSchema } from '../components/output';
import { QueryVariableList } from '../components/query-variable-list';
import { FilterValues } from './filter-values';
import { SelectKeys } from './select-keys';
import { Updates } from './updates';

export const RetrievalPartialSchema = {
  query: z.array(
    z.object({
      input: z.string().min(1, { message: t('flow.queryRequired') }),
    }),
  ),
  operations: z.string(),
  select_keys: z.array(z.object({ name: z.string().optional() })).optional(),
  remove_keys: z.array(z.object({ name: z.string().optional() })).optional(),
  updates: z
    .array(
      z.object({ key: z.string().optional(), value: z.string().optional() }),
    )
    .optional(),
  rename_keys: z
    .array(
      z.object({
        old_key: z.string().optional(),
        new_key: z.string().optional(),
      }),
    )
    .optional(),
  filter_values: z
    .array(
      z.object({
        key: z.string().optional(),
        value: z.string().optional(),
        operator: z.string().optional(),
      }),
    )
    .optional(),
  ...OutputSchema,
};

export const FormSchema = z.object(RetrievalPartialSchema);

export type DataOperationsFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialDataOperationsValues.outputs);

function DataOperationsForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const defaultValues = useFormValues(initialDataOperationsValues, node);

  const form = useForm<DataOperationsFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const operations = useWatch({ control: form.control, name: 'operations' });

  const OperationsOptions = buildOptions(
    Operations,
    t,
    `flow.operationsOptions`,
    true,
  );

  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariableList
          tooltip={t('flow.queryTip')}
          label={t('flow.query')}
          types={[JsonSchemaDataType.Object]}
        ></QueryVariableList>
        <Separator />
        <RAGFlowFormItem name="operations" label={t('flow.operations')}>
          <SelectWithSearch options={OperationsOptions} allowClear />
        </RAGFlowFormItem>
        {operations === Operations.SelectKeys && (
          <SelectKeys
            name="select_keys"
            label={t('flow.operationsOptions.selectKeys')}
          ></SelectKeys>
        )}
        {operations === Operations.RemoveKeys && (
          <SelectKeys
            name="remove_keys"
            label={t('flow.operationsOptions.removeKeys')}
          ></SelectKeys>
        )}
        {operations === Operations.AppendOrUpdate && (
          <Updates
            name="updates"
            label={t('flow.operationsOptions.appendOrUpdate')}
            keyField="key"
            valueField="value"
          ></Updates>
        )}
        {operations === Operations.RenameKeys && (
          <Updates
            name="rename_keys"
            label={t('flow.operationsOptions.renameKeys')}
            keyField="old_key"
            valueField="new_key"
          ></Updates>
        )}
        {operations === Operations.FilterValues && (
          <FilterValues
            name="filter_values"
            label={t('flow.operationsOptions.filterValues')}
          ></FilterValues>
        )}
        <Output list={outputList} isFormRequired></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(DataOperationsForm);

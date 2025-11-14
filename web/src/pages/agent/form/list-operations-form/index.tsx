import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  DataOperationsOperatorOptions,
  JsonSchemaDataType,
  ListOperations,
  SortMethod,
  initialListOperationsValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output, OutputSchema } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';

export const RetrievalPartialSchema = {
  query: z.string(),
  operations: z.string(),
  n: z.number().int().min(0).optional(),
  sort_method: z.string().optional(),
  filter: z
    .object({
      value: z.string().optional(),
      operator: z.string().optional(),
    })
    .optional(),
  ...OutputSchema,
};

export const FormSchema = z.object(RetrievalPartialSchema);

export type ListOperationsFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialListOperationsValues.outputs);

function ListOperationsForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const defaultValues = useFormValues(initialListOperationsValues, node);

  const form = useForm<ListOperationsFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const operations = useWatch({ control: form.control, name: 'operations' });

  const ListOperationsOptions = buildOptions(
    ListOperations,
    t,
    `flow.ListOperationsOptions`,
    true,
  );
  const SortMethodOptions = buildOptions(
    SortMethod,
    t,
    `flow.SortMethodOptions`,
    true,
  );
  const operatorOptions = useBuildSwitchOperatorOptions(
    DataOperationsOperatorOptions,
  );
  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable
          name="query"
          className="flex-1"
          types={[JsonSchemaDataType.Array]}
        ></QueryVariable>
        <Separator />
        <RAGFlowFormItem name="operations" label={t('flow.operations')}>
          <SelectWithSearch options={ListOperationsOptions} />
        </RAGFlowFormItem>
        {[
          ListOperations.TopN,
          ListOperations.Head,
          ListOperations.Tail,
        ].includes(operations as ListOperations) && (
          <FormField
            control={form.control}
            name="n"
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
        )}
        {[ListOperations.Sort].includes(operations as ListOperations) && (
          <RAGFlowFormItem name="sort_method" label={t('flow.sortMethod')}>
            <SelectWithSearch options={SortMethodOptions} />
          </RAGFlowFormItem>
        )}
        {[ListOperations.Filter].includes(operations as ListOperations) && (
          <div className="flex items-center gap-2">
            <RAGFlowFormItem name="filter.operator" className="flex-1">
              <SelectWithSearch options={operatorOptions}></SelectWithSearch>
            </RAGFlowFormItem>
            <Separator className="w-2" />
            <RAGFlowFormItem name="filter.value" className="flex-1">
              <PromptEditor showToolbar={false} multiLine={false} />
            </RAGFlowFormItem>
          </div>
        )}
        <Output list={outputList} isFormRequired></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(ListOperationsForm);

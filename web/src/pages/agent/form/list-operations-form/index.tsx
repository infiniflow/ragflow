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
import { memo, useCallback, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ArrayFields,
  DataOperationsOperatorOptions,
  ListOperations,
  SortMethod,
  initialListOperationsValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { getArrayElementType } from '../../utils';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output, OutputSchema } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';

export const RetrievalPartialSchema = {
  query: z.string(),
  operations: z.string(),
  n: z.number().int().min(1).optional(),
  sort_method: z.string().optional(),
  filter: z
    .object({
      value: z.string().optional(),
      operator: z.string().optional(),
    })
    .optional(),
  ...OutputSchema,
};

const NumFields = [
  ListOperations.TopN,
  ListOperations.Head,
  ListOperations.Tail,
];

function showField(operations: string) {
  const showNum = NumFields.includes(operations as ListOperations);
  const showSortMethod = [ListOperations.Sort].includes(
    operations as ListOperations,
  );
  const showFilter = [ListOperations.Filter].includes(
    operations as ListOperations,
  );

  return {
    showNum,
    showSortMethod,
    showFilter,
  };
}

export const FormSchema = z.object(RetrievalPartialSchema);

export type ListOperationsFormSchemaType = z.infer<typeof FormSchema>;

function ListOperationsForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const { getType } = useGetVariableLabelOrTypeByValue();

  const defaultValues = useFormValues(initialListOperationsValues, node);

  const form = useForm<ListOperationsFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
    // shouldUnregister: true,
  });

  const operations = useWatch({ control: form.control, name: 'operations' });

  const query = useWatch({ control: form.control, name: 'query' });

  const subType = getArrayElementType(getType(query));

  const currentOutputs = useMemo(() => {
    return {
      result: {
        type: `Array<${subType}>`,
      },
      first: {
        type: subType,
      },
      last: {
        type: subType,
      },
    };
  }, [subType]);

  const outputList = buildOutputList(currentOutputs);

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

  const { showFilter, showNum, showSortMethod } = showField(operations);

  const handleOperationsChange = useCallback(
    (operations: string) => {
      const { showFilter, showNum, showSortMethod } = showField(operations);

      if (showNum) {
        form.setValue('n', 1, { shouldDirty: true });
      }

      if (showSortMethod) {
        form.setValue('sort_method', SortMethodOptions.at(0)?.value, {
          shouldDirty: true,
        });
      }
      if (showFilter) {
        form.setValue('filter.operator', operatorOptions.at(0)?.value, {
          shouldDirty: true,
        });
      }
    },
    [SortMethodOptions, form, operatorOptions],
  );

  useEffect(() => {
    form.setValue('outputs', currentOutputs, { shouldDirty: true });
  }, [currentOutputs, form]);

  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable
          name="query"
          className="flex-1"
          types={ArrayFields as any[]}
        ></QueryVariable>
        <Separator />
        <RAGFlowFormItem name="operations" label={t('flow.operations')}>
          {(field) => (
            <SelectWithSearch
              options={ListOperationsOptions}
              value={field.value}
              onChange={(val) => {
                handleOperationsChange(val);
                field.onChange(val);
              }}
            />
          )}
        </RAGFlowFormItem>
        {showNum && (
          <FormField
            control={form.control}
            name="n"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.flowNum')}</FormLabel>
                <FormControl>
                  <NumberInput
                    {...field}
                    className="w-full"
                    min={1}
                  ></NumberInput>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        {showSortMethod && (
          <RAGFlowFormItem name="sort_method" label={t('flow.sortMethod')}>
            <SelectWithSearch options={SortMethodOptions} />
          </RAGFlowFormItem>
        )}
        {showFilter && (
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

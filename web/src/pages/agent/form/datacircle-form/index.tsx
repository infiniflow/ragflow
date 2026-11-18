import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { DatacircleSort, initialDatacircleValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const DatacircleSortLabelKeys: Record<DatacircleSort, string> = {
  [DatacircleSort.Score]: 'datacircleSortScore',
  [DatacircleSort.Price]: 'datacircleSortPrice',
  [DatacircleSort.Provider]: 'datacircleSortProvider',
};

export const DatacircleFormPartialSchema = {
  sort: z.string().optional(),
  top_n: z.coerce.number(),
};

const FormSchema = z.object({
  query: z.string().optional(),
  api: z.string().optional(),
  ...DatacircleFormPartialSchema,
});

export function DatacircleWidgets() {
  const { t } = useTranslate('flow');

  const sortOptions = useMemo(
    () =>
      Object.values(DatacircleSort).map((x) => ({
        value: x,
        label: t(DatacircleSortLabelKeys[x]),
      })),
    [t],
  );

  return (
    <>
      <RAGFlowFormItem
        name="sort"
        label={t('datacircleSort')}
        tooltip={t('datacircleSortTip')}
      >
        <SelectWithSearch options={sortOptions} />
      </RAGFlowFormItem>
      <TopNFormField max={50}></TopNFormField>
    </>
  );
}

const DatacircleOutputList = buildOutputList(initialDatacircleValues.outputs);

function DatacircleForm({ node }: INextOperatorForm) {
  const { t } = useTranslate('flow');
  const defaultValues = useFormValues(initialDatacircleValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <RAGFlowFormItem
            name="api"
            label={t('datacircleApi')}
            tooltip={t('datacircleApiTip')}
          >
            <Input placeholder="Employee email"></Input>
          </RAGFlowFormItem>
          <DatacircleWidgets></DatacircleWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={DatacircleOutputList}></Output>
      </div>
    </Form>
  );
}

export default memo(DatacircleForm);

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialHierarchicalMergerValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialHierarchicalMergerValues.outputs);

enum Hierarchy {
  H1 = '1',
  H2 = '2',
  H3 = '3',
  H4 = '4',
  H5 = '5',
}

const HierarchyOptions = [
  { label: 'H1', value: Hierarchy.H1 },
  { label: 'H2', value: Hierarchy.H2 },
  { label: 'H3', value: Hierarchy.H3 },
  { label: 'H4', value: Hierarchy.H4 },
  { label: 'H5', value: Hierarchy.H5 },
];

export const FormSchema = z.object({
  hierarchy: z.number(),
  levels: z.array(
    z.object({
      expressions: z.array(z.object({ expression: z.string() })),
    }),
  ),
});

export type HierarchicalMergerFormSchemaType = z.infer<typeof FormSchema>;

type RegularExpressionsProps = {
  index: number;
  parentName: string;
  removeParent: (index: number) => void;
};

export function RegularExpressions({
  index,
  parentName,
  removeParent,
}: RegularExpressionsProps) {
  const form = useFormContext();

  const name = `${parentName}.${index}.expressions`;

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <Card>
      <CardHeader className="flex-row justify-between items-center">
        <CardTitle>H{index + 1}</CardTitle>
        <Button
          type="button"
          variant={'ghost'}
          onClick={() => removeParent(index)}
        >
          <X />
        </Button>
      </CardHeader>
      <CardContent>
        <section className="space-y-4">
          {fields.map((field, index) => (
            <div key={field.id} className="flex items-center gap-2">
              <div className="space-y-2 flex-1">
                <RAGFlowFormItem
                  name={`${name}.${index}.expression`}
                  label={'expression'}
                  labelClassName="!hidden"
                >
                  <Input className="!m-0"></Input>
                </RAGFlowFormItem>
              </div>
              <Button
                type="button"
                variant={'ghost'}
                onClick={() => remove(index)}
              >
                <X />
              </Button>
            </div>
          ))}
        </section>
        <BlockButton
          onClick={() => append({ expression: '' })}
          className="mt-6"
        >
          Add
        </BlockButton>
      </CardContent>
    </Card>
  );
}

const HierarchicalMergerForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialHierarchicalMergerValues, node);

  const form = useForm<HierarchicalMergerFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const name = 'levels';

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <RAGFlowFormItem name={'hierarchy'} label={'hierarchy'}>
          <SelectWithSearch options={HierarchyOptions}></SelectWithSearch>
        </RAGFlowFormItem>
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-center gap-2">
            <div className="space-y-2 flex-1">
              <RegularExpressions
                parentName={name}
                index={index}
                removeParent={remove}
              ></RegularExpressions>
            </div>
          </div>
        ))}
        <BlockButton onClick={() => append({ expressions: [] })}>
          Add
        </BlockButton>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(HierarchicalMergerForm);

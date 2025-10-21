import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Form, FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { Plus, Trash2 } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  Hierarchy,
  initialHierarchicalMergerValues,
} from '../../constant/pipeline';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialHierarchicalMergerValues.outputs);

const HierarchyOptions = [
  { label: 'H1', value: Hierarchy.H1 },
  { label: 'H2', value: Hierarchy.H2 },
  { label: 'H3', value: Hierarchy.H3 },
  { label: 'H4', value: Hierarchy.H4 },
  { label: 'H5', value: Hierarchy.H5 },
];

export const FormSchema = z.object({
  hierarchy: z.string(),
  levels: z.array(
    z.object({
      expressions: z.array(
        z.object({
          expression: z.string().refine(
            (val) => {
              try {
                // Try converting the string to a RegExp
                new RegExp(val);
                return true;
              } catch {
                return false;
              }
            },
            {
              message: 'Must be a valid regular expression string',
            },
          ),
        }),
      ),
    }),
  ),
});

export type HierarchicalMergerFormSchemaType = z.infer<typeof FormSchema>;

type RegularExpressionsProps = {
  index: number;
  parentName: string;
  removeParent: (index: number) => void;
  isLatest: boolean;
};

export function RegularExpressions({
  index,
  parentName,
  isLatest,
  removeParent,
}: RegularExpressionsProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const name = `${parentName}.${index}.expressions`;

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <Card>
      <CardHeader className="flex-row justify-between items-center">
        <span>H{index + 1}</span>
        {isLatest && (
          <Button
            type="button"
            variant={'ghost'}
            onClick={() => removeParent(index)}
          >
            <Trash2 />
          </Button>
        )}
      </CardHeader>
      <CardContent>
        <FormLabel required className="mb-2 text-text-secondary">
          {t('flow.regularExpressions')}
        </FormLabel>
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
              {index === 0 ? (
                <Button
                  onClick={() => append({ expression: '' })}
                  variant={'ghost'}
                >
                  <Plus></Plus>
                </Button>
              ) : (
                <Button
                  type="button"
                  variant={'ghost'}
                  onClick={() => remove(index)}
                >
                  <Trash2 />
                </Button>
              )}
            </div>
          ))}
        </section>
      </CardContent>
    </Card>
  );
}

const HierarchicalMergerForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialHierarchicalMergerValues, node);

  const form = useForm<HierarchicalMergerFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
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
        <RAGFlowFormItem name={'hierarchy'} label={t('flow.hierarchy')}>
          <SelectWithSearch options={HierarchyOptions}></SelectWithSearch>
        </RAGFlowFormItem>
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-center">
            <div className="flex-1">
              <RegularExpressions
                parentName={name}
                index={index}
                removeParent={remove}
                isLatest={index === fields.length - 1}
              ></RegularExpressions>
            </div>
          </div>
        ))}
        {fields.length < 5 && (
          <BlockButton
            onClick={() => append({ expressions: [{ expression: '' }] })}
          >
            {t('common.add')}
          </BlockButton>
        )}
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(HierarchicalMergerForm);

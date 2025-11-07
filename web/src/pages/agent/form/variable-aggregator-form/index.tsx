import { BlockButton } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { initialDataOperationsValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { DynamicGroupVariable } from './dynamic-group-variable';

export const RetrievalPartialSchema = {
  groups: z.array(
    z.object({
      group_name: z.string(),
      variables: z.array(z.object({ value: z.string().optional() })),
    }),
  ),
  operations: z.string(),
};

export const FormSchema = z.object(RetrievalPartialSchema);

export type DataOperationsFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialDataOperationsValues.outputs);

function VariableAggregatorForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const defaultValues = useFormValues(initialDataOperationsValues, node);

  const form = useForm<DataOperationsFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const { fields, remove, append } = useFieldArray({
    name: 'groups',
    control: form.control,
  });

  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <section className="divide-y">
          {fields.map((field, idx) => (
            <DynamicGroupVariable
              key={field.id}
              name={`groups.${idx}`}
              parentIndex={idx}
              removeParent={remove}
            ></DynamicGroupVariable>
          ))}
        </section>
        <BlockButton
          onClick={() =>
            append({ group_name: `Group ${fields.length}`, variables: [] })
          }
        >
          {t('common.add')}
        </BlockButton>
        <Separator />

        <Output list={outputList} isFormRequired></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(VariableAggregatorForm);

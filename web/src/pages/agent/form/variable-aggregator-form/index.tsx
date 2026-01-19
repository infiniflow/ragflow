import { BlockButton } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useCallback } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { initialDataOperationsValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { DynamicGroupVariable } from './dynamic-group-variable';
import { FormSchema, VariableAggregatorFormSchemaType } from './schema';
import { useWatchFormChange } from './use-watch-change';

function VariableAggregatorForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const getNode = useGraphStore((state) => state.getNode);

  const defaultValues = useFormValues(initialDataOperationsValues, node);

  const form = useForm<VariableAggregatorFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const { fields, remove, append } = useFieldArray({
    name: 'groups',
    control: form.control,
  });

  const appendItem = useCallback(() => {
    append({ group_name: `Group${fields.length}`, variables: [] });
  }, [append, fields.length]);

  const outputList = buildOutputList(
    getNode(node?.id)?.data.form.outputs ?? {},
  );

  useWatchFormChange(node?.id, form);

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
        <BlockButton onClick={appendItem}>{t('common.add')}</BlockButton>
        <Separator />

        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(VariableAggregatorForm);

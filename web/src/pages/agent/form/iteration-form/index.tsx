import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { ArrayFields } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { DynamicOutput } from './dynamic-output';
import { OutputArray } from './interface';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

const FormSchema = z.object({
  query: z.string().optional(),
  variables: z.array(
    z.object({
      variable: z.string().optional(),
      operator: z.string().optional(),
      parameter: z.string().or(z.number()).or(z.boolean()).optional(),
      mode: z.string(),
    }),
  ),
  outputs: z.array(z.object({ name: z.string(), value: z.any() })).optional(),
});

function IterationForm({ node }: INextOperatorForm) {
  const defaultValues = useValues(node);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  const outputs: OutputArray = useWatch({
    control: form?.control,
    name: 'outputs',
  });

  const outputList = useMemo(() => {
    return outputs.map((x) => ({ title: x.name, type: x?.type }));
  }, [outputs]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable
          name="items_ref"
          types={ArrayFields as any[]}
        ></QueryVariable>
        <DynamicOutput node={node}></DynamicOutput>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(IterationForm);

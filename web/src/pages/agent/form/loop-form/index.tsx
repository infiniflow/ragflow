import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialLoopValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { DynamicVariables } from './dynamic-variables';

const FormSchema = z.object({
  loop_variables: z.array(
    z.object({
      variable: z.string().optional(),
      type: z.string().optional(),
      value: z.string().or(z.number()).or(z.boolean()).optional(),
      input_mode: z.string(),
    }),
  ),
  loop_termination_condition: z.array(
    z.object({
      variable: z.string().optional(),
      operator: z.string().optional(),
      value: z.string().or(z.number()).or(z.boolean()).optional(),
      input_mode: z.string(),
    }),
  ),
});

function LoopForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialLoopValues, node);

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <DynamicVariables
          name="loop_variables"
          label="Variables"
        ></DynamicVariables>
      </FormWrapper>
    </Form>
  );
}

export default memo(LoopForm);

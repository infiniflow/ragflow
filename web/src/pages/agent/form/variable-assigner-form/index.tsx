import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { initialDataOperationsValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { DynamicVariables } from './dynamic-variables';
import { FormSchema, VariableAssignerFormSchemaType } from './schema';

function VariableAssignerForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialDataOperationsValues, node);

  const form = useForm<VariableAssignerFormSchemaType>({
    defaultValues: defaultValues,
    mode: 'onChange',
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form, true);

  return (
    <Form {...form}>
      <FormWrapper>
        <DynamicVariables name="variables" label="Variables"></DynamicVariables>
      </FormWrapper>
    </Form>
  );
}

export default memo(VariableAssignerForm);

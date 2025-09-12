import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { ExeSQLFormWidgets } from '../../exesql-form';
import {
  ExeSQLFormSchema,
  useSubmitForm,
} from '../../exesql-form/use-submit-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

const FormSchema = z.object(ExeSQLFormSchema);

type FormType = z.infer<typeof FormSchema>;

const ExeSQLForm = () => {
  const { onSubmit, loading } = useSubmitForm();

  const defaultValues = useValues();

  const form = useForm<FormType>({
    resolver: zodResolver(FormSchema),
    defaultValues: defaultValues as FormType,
  });

  const onError = (error: any) => console.log(error);

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper onSubmit={form.handleSubmit(onSubmit, onError)}>
        <ExeSQLFormWidgets loading={loading}></ExeSQLFormWidgets>
      </FormWrapper>
    </Form>
  );
};

export default ExeSQLForm;

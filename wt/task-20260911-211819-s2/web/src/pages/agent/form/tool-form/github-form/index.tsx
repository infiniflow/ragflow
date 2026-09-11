import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function GithubForm() {
  const values = useValues();

  const FormSchema = z.object({ query: z.string() });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <TopNFormField></TopNFormField>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(GithubForm);

import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { PubMedFormPartialSchema, PubMedFormWidgets } from '../../pubmed-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function PubMedForm() {
  const values = useValues();

  const FormSchema = z.object(PubMedFormPartialSchema);

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
          <PubMedFormWidgets></PubMedFormWidgets>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(PubMedForm);

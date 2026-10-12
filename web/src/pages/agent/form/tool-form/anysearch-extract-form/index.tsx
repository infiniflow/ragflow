import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  AnySearchExtractFormPartialSchema,
  AnySearchExtractWidgets,
} from '../../anysearch-extract-form';
import { FormWrapper } from '../../components/form-wrapper';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function AnySearchExtractForm() {
  const values = useValues();

  const FormSchema = z.object(AnySearchExtractFormPartialSchema);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <AnySearchExtractWidgets></AnySearchExtractWidgets>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(AnySearchExtractForm);

import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { BingFormSchema, BingFormWidgets } from '../../bing-form';
import { FormWrapper } from '../../components/form-wrapper';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

export const FormSchema = z.object(BingFormSchema);

function BingForm() {
  const defaultValues = useValues();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues,
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <BingFormWidgets></BingFormWidgets>
      </FormWrapper>
    </Form>
  );
}

export default memo(BingForm);

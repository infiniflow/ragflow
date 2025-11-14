import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { ApiKeyField } from '../../components/api-key-field';
import { FormWrapper } from '../../components/form-wrapper';
import { TavilyFormSchema } from '../../tavily-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function TavilyForm() {
  const values = useValues();

  const FormSchema = z.object(TavilyFormSchema);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <ApiKeyField></ApiKeyField>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(TavilyForm);

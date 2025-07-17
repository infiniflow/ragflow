import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { TavilyApiKeyField, TavilyFormSchema } from '../../tavily-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

const TavilyForm = () => {
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
          <TavilyApiKeyField></TavilyApiKeyField>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
};

export default TavilyForm;

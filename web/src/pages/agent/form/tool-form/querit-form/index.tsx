import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { QueritFormWidgets } from '../../querit-form';
import { useValues } from '../use-values';
import { useQueritAgentFormChange } from './use-watch-change';
import {
  QueritAgentFormSchema,
  deserializeQueritAgentFormValues,
} from './utils';

function QueritForm() {
  const persistedValues = useValues();
  const values = useMemo(
    () => deserializeQueritAgentFormValues(persistedValues),
    [persistedValues],
  );
  const form = useForm<z.infer<typeof QueritAgentFormSchema>>({
    defaultValues: values,
    resolver: zodResolver(QueritAgentFormSchema),
    mode: 'onChange',
  });

  useQueritAgentFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueritFormWidgets includeQuery={false} />
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(QueritForm);

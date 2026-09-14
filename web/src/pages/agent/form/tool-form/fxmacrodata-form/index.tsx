import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import {
  FXMacroDataFormPartialSchema,
  FXMacroDataWidgets,
} from '../../fxmacrodata-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function FXMacroDataForm() {
  const values = useValues();
  const schema = z.object(FXMacroDataFormPartialSchema);
  const form = useForm<z.infer<typeof schema>>({
    defaultValues: values,
    resolver: zodResolver(schema),
  });
  useWatchFormChange(form);
  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <FXMacroDataWidgets agentTool />
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(FXMacroDataForm);

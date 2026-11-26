import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import {
  SpecStreamWidgets,
  useSpecStreamFormPartialSchema,
} from '../../specstream-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function SpecStreamForm() {
  const values = useValues();

  const partialSchema = useSpecStreamFormPartialSchema();
  const FormSchema = useMemo(() => z.object(partialSchema), [partialSchema]);

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
          <SpecStreamWidgets></SpecStreamWidgets>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(SpecStreamForm);

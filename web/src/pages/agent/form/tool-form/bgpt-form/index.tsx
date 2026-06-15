import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { FormWrapper } from '../../components/form-wrapper';
import { BGPTFormPartialSchema, BGPTFormWidgets } from '../../bgpt-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

function BGPTToolForm() {
  const values = useValues();
  const { t } = useTranslate('flow');

  const FormSchema = z.object(BGPTFormPartialSchema);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <BGPTFormWidgets></BGPTFormWidgets>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(BGPTToolForm);

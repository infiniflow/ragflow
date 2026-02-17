import { LargeModelFormFieldWithoutFilter } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { Form } from '@/components/ui/form';
import { useFetchDialog } from '@/hooks/use-chat-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

export function LLMSelectForm() {
  const FormSchema = z.object(LlmSettingSchema);
  const { data } = useFetchDialog();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      llm_id: '',
    },
  });

  // const values = useWatch({ control: form.control, name: ['llm_id'] });

  useEffect(() => {
    if (!isEmpty(data)) {
      form.reset({ llm_id: data.llm_id, ...data.llm_setting });
    }
    form.reset(data);
  }, [data, form]);

  return (
    <Form {...form}>
      <LargeModelFormFieldWithoutFilter></LargeModelFormFieldWithoutFilter>
    </Form>
  );
}

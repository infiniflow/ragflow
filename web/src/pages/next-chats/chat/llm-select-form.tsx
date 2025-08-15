import { LargeModelFormFieldWithoutFilter } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

export function LLMSelectForm() {
  const FormSchema = z.object(LlmSettingSchema);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      llm_id: '',
    },
  });

  return (
    <Form {...form}>
      <LargeModelFormFieldWithoutFilter></LargeModelFormFieldWithoutFilter>
    </Form>
  );
}

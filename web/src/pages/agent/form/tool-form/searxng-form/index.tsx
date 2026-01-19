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
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

const FormSchema = z.object({
  searxng_url: z.string().min(1),
  top_n: z.string(),
});

function SearXNGForm() {
  const { t } = useTranslate('flow');
  const values = useValues();

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values as any,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormContainer>
        <TopNFormField></TopNFormField>
        <FormField
          control={form.control}
          name="searxng_url"
          render={({ field }) => (
            <FormItem>
              <FormLabel>SearXNG URL</FormLabel>
              <FormControl>
                <Input {...field} placeholder="http://localhost:4000" />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </FormContainer>
    </Form>
  );
}

export default memo(SearXNGForm);

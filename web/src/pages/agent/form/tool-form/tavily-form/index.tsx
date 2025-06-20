import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

const TavilyForm = () => {
  const values = useValues();

  const FormSchema = z.object({
    api_key: z.string(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <form
        className="space-y-5 px-5 "
        autoComplete="off"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <FormField
            control={form.control}
            name="api_key"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Api Key</FormLabel>
                <FormControl>
                  <Input type="password" {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
      </form>
    </Form>
  );
};

export default TavilyForm;

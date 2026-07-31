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
import { useCallback, useId, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

interface CreateEmailFormData {
  email: string;
}

interface CreateEmailFormProps {
  id: string;
  form: ReturnType<typeof useForm<CreateEmailFormData>>;
  onSubmit?: (data: CreateEmailFormData) => void;
}

export const CreateEmailForm = ({
  id,
  form,
  onSubmit = () => {},
}: CreateEmailFormProps) => {
  const { t } = useTranslation();

  return (
    <Form {...form}>
      <form
        id={id}
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {/* Email field */}
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.email')}
              </FormLabel>
              <FormControl>
                <Input
                  placeholder="name@example.com"
                  autoComplete="email"
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
};

// Export the form validation state for parent component
function useCreateEmailForm(props?: {
  defaultValues: Partial<CreateEmailFormData>;
}) {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z.object({
      email: z.string().email({ message: t('admin.invalidEmail') }),
    });
  }, [t]);

  const form = useForm<CreateEmailFormData>({
    defaultValues: {
      email: '',
      ...(props?.defaultValues ?? {}),
    },
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<CreateEmailFormProps>) => (
      <CreateEmailForm id={id} form={form} {...props} />
    ),
    [id, form],
  );

  return {
    schema,
    id,
    form,
    FormComponent,
  };
}

export default useCreateEmailForm;

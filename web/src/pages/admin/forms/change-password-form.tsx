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

interface ChangePasswordFormData {
  newPassword: string;
  confirmPassword: string;
}

interface ChangePasswordFormProps {
  id: string;
  form: ReturnType<typeof useForm<ChangePasswordFormData>>;
  email?: string;
  onSubmit?: (data: ChangePasswordFormData) => void;
}

export const ChangePasswordForm = ({
  id,
  form,
  email,
  onSubmit = () => {},
}: ChangePasswordFormProps) => {
  const { t } = useTranslation();

  return (
    <Form {...form}>
      <form
        id={id}
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {/* Email field (readonly) */}
        <div>
          <FormLabel className="text-sm font-medium">
            {t('admin.email')}
          </FormLabel>
          <Input
            value={email}
            readOnly
            className="mt-2 px-3 h-10 bg-bg-input border-border-button"
          />
        </div>

        {/* New password field */}
        <FormField
          control={form.control}
          name="newPassword"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.newPassword')}
              </FormLabel>

              <FormControl>
                <Input
                  type="password"
                  placeholder={t('admin.newPassword')}
                  autoComplete="new-password"
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Confirm password field */}
        <FormField
          control={form.control}
          name="confirmPassword"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.confirmNewPassword')}
              </FormLabel>
              <FormControl>
                <Input
                  type="password"
                  placeholder={t('admin.confirmNewPassword')}
                  autoComplete="new-password"
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
function useChangePasswordForm() {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z
      .object({
        newPassword: z
          .string()
          .min(8, { message: t('admin.passwordMinLength') }),
        confirmPassword: z
          .string()
          .min(8, { message: t('admin.confirmPasswordRequired') }),
      })
      .refine((data) => data.newPassword === data.confirmPassword, {
        message: t('admin.confirmPasswordDoNotMatch'),
        path: ['confirmPassword'],
      });
  }, [t]);

  const form = useForm<ChangePasswordFormData>({
    defaultValues: {
      newPassword: '',
      confirmPassword: '',
    },
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<ChangePasswordFormProps>) => (
      <ChangePasswordForm id={id} form={form} {...props} />
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

export default useChangePasswordForm;

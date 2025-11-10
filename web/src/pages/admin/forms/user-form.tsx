import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useId, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { listRoles } from '@/services/admin-service';

import EnterpriseFeature from '../components/enterprise-feature';
import { IS_ENTERPRISE } from '../utils';

interface CreateUserFormData {
  email: string;
  password: string;
  confirmPassword: string;
  role?: string;
}

interface CreateUserFormProps {
  id: string;
  form: ReturnType<typeof useForm<CreateUserFormData>>;
  onSubmit?: (data: CreateUserFormData) => void;
}

export const CreateUserForm = ({
  id,
  form,
  onSubmit = () => {},
}: CreateUserFormProps) => {
  const { t } = useTranslation();

  const { data: roleList } = useQuery({
    queryKey: ['admin/listRoles'],
    queryFn: async () => (await listRoles()).data.data.roles,
    enabled: IS_ENTERPRISE,
    retry: false,
  });

  return (
    <Form {...form}>
      <form
        id={id}
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {/* Email field (editable) */}
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
                  placeholder={t('admin.email')}
                  autoComplete="username"
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Password field */}
        <FormField
          control={form.control}
          name="password"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.password')}
              </FormLabel>
              <FormControl>
                <Input
                  type="password"
                  placeholder={t('admin.password')}
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
                {t('admin.confirmPassword')}
              </FormLabel>
              <FormControl>
                <Input
                  type="password"
                  placeholder={t('admin.confirmPassword')}
                  autoComplete="new-password"
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <EnterpriseFeature>
          {/* Role field */}
          {() => (
            <FormField
              control={form.control}
              name="role"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-sm font-medium">
                    {t('admin.role')}
                  </FormLabel>
                  <FormControl>
                    <Select {...field}>
                      <SelectTrigger className="w-full h-10">
                        <SelectValue />
                      </SelectTrigger>

                      <SelectContent className="bg-bg-base">
                        <SelectGroup>
                          {roleList?.map((role) => (
                            <SelectItem key={role.id} value={role.role_name}>
                              {role.role_name}
                            </SelectItem>
                          )) ?? (
                            <div className="text-text-secondary px-2 py-6 text-sm text-center">
                              {t('common.noData')}
                            </div>
                          )}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </FormControl>
                </FormItem>
              )}
            />
          )}
        </EnterpriseFeature>
      </form>
    </Form>
  );
};

// Export the form validation state for parent component
function useCreateUserForm(props?: {
  defaultValues: Partial<CreateUserFormData>;
}) {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z
      .object({
        email: z.string().email({ message: t('admin.invalidEmail') }),
        password: z.string().min(6, { message: t('admin.passwordMinLength') }),
        confirmPassword: z
          .string()
          .min(1, { message: t('admin.confirmPasswordRequired') }),
        role: z.string().optional(),
      })
      .refine((data) => data.password === data.confirmPassword, {
        message: t('admin.confirmPasswordDoNotMatch'),
        path: ['confirmPassword'],
      });
  }, [t]);

  const form = useForm<CreateUserFormData>({
    defaultValues: {
      email: '',
      password: '',
      confirmPassword: '',
      ...(props?.defaultValues ?? {}),
    },
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<CreateUserFormProps>) => (
      <CreateUserForm id={id} form={form} {...props} />
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

export default useCreateUserForm;

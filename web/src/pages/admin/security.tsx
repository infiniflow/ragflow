import { useContext, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useForm } from 'react-hook-form';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { changeOwnPassword } from '@/services/admin-service';
import { adminStorage } from '@/utils/authorization-util';
import { rsaPsw } from '@/utils';
import { Routes } from '@/routes';
import { CurrentUserInfoContext } from './layouts/root-layout';

interface PasswordValues {
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
}

const PasswordFields = [
  {
    name: 'currentPassword',
    label: 'adminEnhancements.currentPassword',
    autoComplete: 'current-password',
  },
  {
    name: 'newPassword',
    label: 'admin.newPassword',
    autoComplete: 'new-password',
  },
  {
    name: 'confirmPassword',
    label: 'admin.confirmNewPassword',
    autoComplete: 'new-password',
  },
] as const;

export default function AdminSecurity() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [, setCurrentUserInfo] = useContext(CurrentUserInfoContext);
  const schema = useMemo(
    () =>
      z
        .object({
          currentPassword: z
            .string()
            .min(1, t('adminEnhancements.currentPasswordRequired')),
          newPassword: z
            .string()
            .min(8, t('admin.passwordMinLength'))
            .max(128, t('adminEnhancements.passwordMaxLength')),
          confirmPassword: z
            .string()
            .min(1, t('admin.confirmPasswordRequired')),
        })
        .refine((value) => value.newPassword === value.confirmPassword, {
          message: t('admin.confirmPasswordDoNotMatch'),
          path: ['confirmPassword'],
        }),
    [t],
  );
  const form = useForm<PasswordValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      currentPassword: '',
      newPassword: '',
      confirmPassword: '',
    },
  });
  const mutation = useMutation({
    mutationFn: (values: PasswordValues) => {
      const currentPassword = rsaPsw(values.currentPassword);
      const newPassword = rsaPsw(values.newPassword);
      if (!currentPassword || !newPassword) {
        throw new Error(t('adminEnhancements.passwordEncryptionFailed'));
      }
      return changeOwnPassword(currentPassword, newPassword);
    },
    onError: (error: Error) => {
      form.setError('root.server', { message: error.message });
    },
    onSuccess: () => {
      form.reset();
      adminStorage.removeAll();
      setCurrentUserInfo({ userInfo: null, source: null });
      queryClient.removeQueries({
        predicate: ({ queryKey }) =>
          typeof queryKey[0] === 'string' &&
          (queryKey[0] === 'admin' || queryKey[0].startsWith('admin/')),
      });
      navigate(Routes.Admin, { replace: true });
    },
  });
  function handleSubmit(values: PasswordValues) {
    form.clearErrors('root');
    mutation.mutate(values);
  }
  return (
    <section className="h-full p-8 overflow-auto">
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle>{t('adminEnhancements.security')}</CardTitle>
          <p className="text-text-secondary">
            {t('adminEnhancements.passwordChangeHint')}
          </p>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form
              className="space-y-6"
              onSubmit={form.handleSubmit(handleSubmit)}
            >
              {PasswordFields.map(({ name, label, autoComplete }) => (
                <FormField
                  key={name}
                  control={form.control}
                  name={name}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t(label)}</FormLabel>
                      <FormControl>
                        <Input
                          type="password"
                          autoComplete={autoComplete}
                          maxLength={
                            name === 'currentPassword' ? undefined : 128
                          }
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ))}
              {form.formState.errors.root?.server?.message && (
                <p role="alert" className="text-text-secondary">
                  {form.formState.errors.root.server.message}
                </p>
              )}
              <Button
                type="submit"
                disabled={mutation.isPending}
                loading={mutation.isPending}
              >
                {t('admin.changePassword')}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </section>
  );
}

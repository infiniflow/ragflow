import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useLoginWithLdap } from '@/hooks/use-login-request';
import { rsaPsw } from '@/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { z } from 'zod';

interface LdapDialogProps {
  open: boolean;
  channel: string | null;
  displayName?: string;
  onOpenChange: (open: boolean) => void;
}

export function LdapDialog({
  open,
  channel,
  displayName,
  onOpenChange,
}: LdapDialogProps) {
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const navigate = useNavigate();
  const { login, loading } = useLoginWithLdap();

  const schema = z.object({
    username: z.string().min(1, { message: t('usernamePlaceholder') }),
    password: z.string().min(1, { message: t('passwordPlaceholder') }),
  });

  type Values = z.infer<typeof schema>;

  const form = useForm<Values>({
    defaultValues: { username: '', password: '' },
    resolver: zodResolver(schema),
  });

  useEffect(() => {
    if (!open) {
      form.reset();
    }
  }, [open, form]);

  const onSubmit = async ({ username, password }: Values) => {
    if (!channel) return;
    const code = await login({
      channel,
      username: username.trim(),
      password: rsaPsw(password) as string,
    });
    if (code === 0) {
      onOpenChange(false);
      navigate('/');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('ldapTitle', { provider: displayName || 'LDAP' })}
          </DialogTitle>
          <DialogDescription>{t('ldapDescription')}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            className="flex flex-col gap-6"
            onSubmit={form.handleSubmit(onSubmit)}
            data-testid="ldap-form"
          >
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('usernameLabel')}</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="ldap-username"
                      placeholder={t('usernamePlaceholder')}
                      autoComplete="username"
                      autoFocus
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('passwordLabel')}</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="ldap-password"
                      type="password"
                      placeholder={t('passwordPlaceholder')}
                      autoComplete="current-password"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t('cancel')}
              </Button>
              <ButtonLoading
                type="submit"
                loading={loading}
                data-testid="ldap-submit"
              >
                {t('login')}
              </ButtonLoading>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

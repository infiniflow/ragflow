import { type AxiosResponseHeaders } from 'axios';
import { useContext, useEffect, useId } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

import { useMutation } from '@tanstack/react-query';

import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardFooter } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Authorization } from '@/constants/authorization';

import { useAuth } from '@/hooks/auth-hooks';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { rsaPsw } from '@/utils';
import authorizationUtil from '@/utils/authorization-util';

import { login } from '@/services/admin-service';

import { BgSvg } from '../login-next/bg';
import ThemeSwitch from './components/theme-switch';

import { CurrentUserInfoContext } from './layouts/root-layout';

function AdminLogin() {
  const navigate = useNavigate();
  const [, setCurrentUserInfo] = useContext(CurrentUserInfoContext);
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const { isLogin } = useAuth();

  const loginMutation = useMutation({
    mutationKey: ['adminLogin'],
    mutationFn: async (params: { email: string; password: string }) => {
      const rsaPassWord = rsaPsw(params.password) as string;
      return await login({
        email: params.email,
        password: rsaPassWord,
      });
    },
    onSuccess: (request) => {
      const { data: req, headers } = request;

      if (req?.code === 0) {
        const authorization = (headers as AxiosResponseHeaders)?.get(
          Authorization,
        );
        const token = req.data.access_token;

        // Lift to global user info context
        setCurrentUserInfo({
          userInfo: req.data,
          source: 'serverRequest',
        });

        authorizationUtil.setItems({
          Authorization: authorization as string,
          Token: token,
          userInfo: JSON.stringify({
            ...req.data,
            name: req.data.nickname,
          }),
        });

        navigate('/admin/services');
      }
    },
    onError: (error) => {
      console.log('Failed:', error);
    },
    retry: false,
  });

  const loading = loginMutation.isPending;

  useEffect(() => {
    if (isLogin) {
      navigate(Routes.AdminServices);
    }
  }, [isLogin, navigate]);

  const FormSchema = z.object({
    email: z
      .string()
      .email()
      .min(1, { message: t('emailPlaceholder') }),
    password: z.string().min(1, { message: t('passwordPlaceholder') }),
    remember: z.boolean().optional(),
  });

  const formId = useId();
  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
      remember: false,
    },
    resolver: zodResolver(FormSchema),
  });

  return (
    <ScrollArea className="w-screen h-screen">
      <div className="relative h-max min-h-[100vh]">
        <Spotlight opcity={0.4} coverage={60} color="rgb(128, 255, 248)" />
        <Spotlight
          opcity={0.3}
          coverage={12}
          X="10%"
          Y="-10%"
          color="rgb(128, 255, 248)"
        />
        <Spotlight
          opcity={0.3}
          coverage={12}
          X="90%"
          Y="-10%"
          color="rgb(128, 255, 248)"
        />

        <BgSvg isPaused={true} />

        <div className="absolute top-3 left-0 w-full">
          <div className="absolute mt-12 ml-12 flex items-center">
            <img className="size-8 mr-5" src="/logo.svg" alt="logo" />
            <span className="text-xl font-bold">RAGFlow</span>
          </div>

          <h1 className="mt-[6.5rem] text-4xl font-medium text-center mb-12">
            {t('loginTitle', { keyPrefix: 'admin' })}
          </h1>
        </div>

        <div className="flex items-center justify-center w-screen">
          <div className="w-full max-w-[540px] mt-72 mb-48">
            <Card className="w-full bg-bg-component rounded-2xl shadow-none backdrop-blur-sm">
              <CardContent className="px-10 pt-14 pb-10">
                <Form {...form}>
                  <form
                    id={formId}
                    className="space-y-8 text-text-primary"
                    onSubmit={form.handleSubmit((data) =>
                      loginMutation.mutate(data),
                    )}
                  >
                    <FormField
                      control={form.control}
                      name="email"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel required>{t('emailLabel')}</FormLabel>

                          <FormControl>
                            <Input
                              className="h-10"
                              placeholder={t('emailPlaceholder')}
                              autoComplete="email"
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
                              {...field}
                              className="h-10"
                              type="password"
                              placeholder={t('passwordPlaceholder')}
                              autoComplete="password"
                            />
                          </FormControl>

                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name="remember"
                      render={({ field }) => (
                        <FormItem className="!mt-5">
                          <FormLabel
                            className={cn(
                              'transition-colors',
                              field.value
                                ? 'text-text-primary'
                                : 'text-text-secondary',
                            )}
                          >
                            <FormControl>
                              <Checkbox
                                checked={field.value}
                                onCheckedChange={field.onChange}
                              />
                            </FormControl>

                            <span className="ml-2">{t('rememberMe')}</span>
                          </FormLabel>
                        </FormItem>
                      )}
                    />
                  </form>
                </Form>
              </CardContent>

              <CardFooter className="px-10 pt-8 pb-14">
                <Button
                  form={formId}
                  variant="highlighted"
                  size="lg"
                  block
                  type="submit"
                  className="font-medium"
                  loading={loading}
                >
                  {t('login')}
                </Button>
              </CardFooter>
            </Card>

            <div className="mt-8 flex justify-center">
              <ThemeSwitch />
            </div>
          </div>
        </div>
      </div>
    </ScrollArea>
  );
}

export default AdminLogin;

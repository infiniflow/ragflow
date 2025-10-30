import { type AxiosResponseHeaders } from 'axios';
import { useEffect, useId, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import { LucideEye, LucideEyeOff } from 'lucide-react';

import { useMutation } from '@tanstack/react-query';

import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';

import Spotlight from '@/components/spotlight';
import { ButtonLoading } from '@/components/ui/button';
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

function AdminLogin() {
  const navigate = useNavigate();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const { isLogin } = useAuth();

  const [showPassword, setShowPassword] = useState(false);

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

        const userInfo = {
          avatar: req.data.avatar,
          name: req.data.nickname,
          email: req.data.email,
        };

        authorizationUtil.setItems({
          Authorization: authorization as string,
          Token: token,
          userInfo: JSON.stringify(userInfo),
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
      <div className="relative">
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

        <BgSvg />

        <div className="absolute top-3 left-0 w-full">
          <div className="absolute mt-12 ml-12 flex items-center">
            <img className="size-8 mr-5" src="/logo.svg" alt="logo" />
            <span className="text-xl font-bold">RAGFlow</span>
          </div>

          <h1 className="mt-[6.5rem] text-4xl font-medium text-center mb-12">
            {t('loginTitle', { keyPrefix: 'admin' })}
          </h1>
        </div>

        <div className="flex items-center justify-center w-screen min-h-[1050px]">
          <div className="w-full max-w-[540px]">
            <Card className="w-full bg-bg-component backdrop-blur-sm rounded-2xl border border-border-button">
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
                              className="h-10 px-2.5"
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
                            <div className="relative">
                              <Input
                                className="h-10 px-2.5"
                                type={showPassword ? 'text' : 'password'}
                                placeholder={t('passwordPlaceholder')}
                                autoComplete="password"
                                {...field}
                              />
                              <button
                                type="button"
                                className="absolute inset-y-0 right-0 pr-3 flex items-center"
                                onClick={() => setShowPassword(!showPassword)}
                              >
                                {showPassword ? (
                                  <LucideEyeOff className="h-4 w-4 text-gray-500" />
                                ) : (
                                  <LucideEye className="h-4 w-4 text-gray-500" />
                                )}
                              </button>
                            </div>
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
                              'flex items-center hover:text-text-primary',
                              field.value
                                ? 'text-text-primary'
                                : 'text-text-disabled',
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
                <ButtonLoading
                  form={formId}
                  size="lg"
                  className="
                    w-full h-10
                    bg-metallic-gradient border-b-[#00BEB4] border-b-2
                    hover:bg-metallic-gradient hover:border-b-[#02bcdd]
                  "
                  type="submit"
                  loading={loading}
                >
                  {t('login')}
                </ButtonLoading>
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

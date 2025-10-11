import SvgIcon from '@/components/svg-icon';
import { useAuth } from '@/hooks/auth-hooks';
import {
  useLogin,
  useLoginChannels,
  useLoginWithChannel,
  useRegister,
} from '@/hooks/login-hooks';
import { useSystemConfig } from '@/hooks/system-hooks';
import { rsaPsw } from '@/utils';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import Spotlight from '@/components/spotlight';
import { Button, ButtonLoading } from '@/components/ui/button';
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
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { BgSvg } from './bg';
import './index.less';
import { SpotlightTopLeft, SpotlightTopRight } from './spotlight-top';

const Login = () => {
  const [title, setTitle] = useState('login');
  const navigate = useNavigate();
  const { login, loading: signLoading } = useLogin();
  const { register, loading: registerLoading } = useRegister();
  const { channels, loading: channelsLoading } = useLoginChannels();
  const { login: loginWithChannel, loading: loginWithChannelLoading } =
    useLoginWithChannel();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const loading =
    signLoading ||
    registerLoading ||
    channelsLoading ||
    loginWithChannelLoading;
  const { config } = useSystemConfig();
  const registerEnabled = config?.registerEnabled !== 0;

  const { isLogin } = useAuth();
  useEffect(() => {
    if (isLogin) {
      navigate('/');
    }
  }, [isLogin, navigate]);

  const handleLoginWithChannel = async (channel: string) => {
    await loginWithChannel(channel);
  };

  const changeTitle = () => {
    if (title === 'login' && !registerEnabled) {
      return;
    }
    setTitle((title) => (title === 'login' ? 'register' : 'login'));
  };

  const FormSchema = z
    .object({
      nickname: z.string().optional(),
      email: z
        .string()
        .email()
        .min(1, { message: t('emailPlaceholder') }),
      password: z.string().min(1, { message: t('passwordPlaceholder') }),
      remember: z.boolean().optional(),
    })
    .superRefine((data, ctx) => {
      if (title === 'register' && !data.nickname) {
        ctx.addIssue({
          path: ['nickname'],
          message: 'nicknamePlaceholder',
          code: z.ZodIssueCode.custom,
        });
      }
    });
  const form = useForm({
    defaultValues: {
      nickname: '',
      email: '',
      password: '',
      confirmPassword: '',
      remember: false,
    },
    resolver: zodResolver(FormSchema),
  });

  const onCheck = async (params) => {
    console.log('params', params);
    try {
      // const params = await form.validateFields();

      const rsaPassWord = rsaPsw(params.password) as string;

      if (title === 'login') {
        const code = await login({
          email: `${params.email}`.trim(),
          password: rsaPassWord,
        });
        if (code === 0) {
          navigate('/');
        }
      } else {
        const code = await register({
          nickname: params.nickname,
          email: params.email,
          password: rsaPassWord,
        });
        if (code === 0) {
          setTitle('login');
        }
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <div className="min-h-screen relative overflow-hidden">
      <BgSvg />
      <Spotlight opcity={0.6} coverage={60} />
      <SpotlightTopLeft opcity={0.2} coverage={20} />
      <SpotlightTopRight opcity={0.2} coverage={20} />
      <div className="absolute top-3 flex flex-col items-center mb-12 w-full text-text-primary">
        <div className="flex items-center mb-4 w-full pl-10 pt-10 ">
          <div className="w-10 h-10 rounded-lg border flex items-center justify-center mr-3">
            <img
              src={'/logo.svg'}
              alt="logo"
              className="size-10 mr-[12] cursor-pointer"
            />
          </div>
          <span className="text-xl font-bold self-end">RAGFlow</span>
        </div>
        <h1 className="text-2xl font-bold  text-center mb-2">
          A Leading RAG engine with Agent for superior LLM context.
        </h1>
        <div className="mt-4 px-6 py-1 text-sm font-medium text-cyan-600 border border-accent-primary rounded-full hover:bg-cyan-50 transition-colors duration-200 border-glow relative overflow-hidden">
          Let's get started
        </div>
      </div>
      <div className="relative z-10 flex flex-col items-center justify-center min-h-screen px-4 sm:px-6 lg:px-8">
        {/* Logo and Header */}

        {/* Login Form */}
        <div className="text-center mb-8">
          <h2 className="text-xl font-semibold text-text-primary">
            {title === 'login'
              ? 'Sign in to Your Account'
              : 'Create an Account'}
          </h2>
        </div>
        <div className="w-full max-w-md bg-bg-base backdrop-blur-sm rounded-2xl shadow-xl p-8 border border-border-button">
          <Form {...form}>
            <form
              className="space-y-6"
              onSubmit={form.handleSubmit((data) => onCheck(data))}
            >
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('emailLabel')}</FormLabel>
                    <FormControl>
                      <Input placeholder={t('emailPlaceholder')} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {title === 'register' && (
                <FormField
                  control={form.control}
                  name="nickname"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel required>{t('nicknameLabel')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('nicknamePlaceholder')}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('passwordLabel')}</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        placeholder={t('passwordPlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {title === 'login' && (
                <FormField
                  control={form.control}
                  name="remember"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <div className="flex gap-2">
                          <Checkbox
                            checked={field.value}
                            onCheckedChange={(checked) => {
                              field.onChange(checked);
                            }}
                          />
                          <FormLabel>{t('rememberMe')}</FormLabel>
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              <ButtonLoading
                type="submit"
                loading={loading}
                className="bg-metallic-gradient border-b-[#00BEB4] border-b-2 hover:bg-metallic-gradient hover:border-b-[#02bcdd] w-full"
              >
                {title === 'login' ? t('login') : t('continue')}
              </ButtonLoading>
              {title === 'login' && channels && channels.length > 0 && (
                <div className="mt-3 border">
                  {channels.map((item) => (
                    <Button
                      variant={'transparent'}
                      key={item.channel}
                      onClick={() => handleLoginWithChannel(item.channel)}
                      style={{ marginTop: 10 }}
                    >
                      <div className="flex items-center">
                        <SvgIcon
                          name={item.icon || 'sso'}
                          width={20}
                          height={20}
                          style={{ marginRight: 5 }}
                        />
                        Sign in with {item.display_name}
                      </div>
                    </Button>
                  ))}
                </div>
              )}
            </form>
          </Form>

          {title === 'login' && registerEnabled && (
            <div className="mt-6 text-right">
              <p className="text-text-disabled text-sm">
                {t('signInTip')}
                <Button
                  variant={'transparent'}
                  onClick={changeTitle}
                  className="text-cyan-600 hover:text-cyan-800 font-medium border-none transition-colors duration-200"
                >
                  {t('signUp')}
                </Button>
              </p>
            </div>
          )}
          {title === 'register' && (
            <div className="mt-6 text-right">
              <p className="text-text-disabled text-sm">
                {t('signUpTip')}
                <Button
                  variant={'transparent'}
                  onClick={changeTitle}
                  className="text-cyan-600 hover:text-cyan-800 font-medium border-none transition-colors duration-200"
                >
                  {t('login')}
                </Button>
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default Login;

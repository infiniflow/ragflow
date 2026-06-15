import SvgIcon from '@/components/svg-icon';
import { useAuth } from '@/hooks/auth-hooks';
import {
  useLogin,
  useLoginChannels,
  useLoginWithChannel,
  useRegister,
} from '@/hooks/use-login-request';
import { useSystemConfig } from '@/hooks/use-system-request';
import { rsaPsw } from '@/utils';
import { useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

import Spotlight from '@/components/spotlight';
import ThemeLogo from '@/components/theme-logo';
import ThemeSwitch from '@/components/theme-switch';
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
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, UseFormReturn } from 'react-hook-form';
import { z } from 'zod';
import FlipCard3D, { FlipFaceContext } from './card';
import './index.less';
type LoginFormContentProps = {
  isLoginPage: boolean;
  title: string;
  form: UseFormReturn<any>;
  loading: boolean;
  onCheck: (params: any) => Promise<void>;
  changeTitle: () => void;
  registerEnabled: boolean;
  channels: { channel: string; icon?: string; display_name: string }[];
  handleLoginWithChannel: (channel: string) => void;
  t: ReturnType<typeof useTranslation>['t'];
  disablePasswordLogin?: boolean;
};

function LoginFormContent({
  isLoginPage,
  title,
  form,
  loading,
  onCheck,
  changeTitle,
  registerEnabled,
  channels,
  handleLoginWithChannel,
  t,
  disablePasswordLogin,
}: LoginFormContentProps) {
  const face = useContext(FlipFaceContext);
  const isActiveFace = isLoginPage ? face === 'front' : face === 'back';

  const pageTitle =
    title === 'login'
      ? 'Sign in to MetaGross-AI'
      : 'Create your MetaGross-AI account';
  const pageDescription =
    title === 'login'
      ? 'Use your credentials to access the AI workspace and manage your workflows.'
      : 'Join MetaGross-AI to start using agents, automations, and smart dashboards.';

  return (
    <div className="flex h-full w-full items-center justify-center">
      <div className="w-full max-w-[540px] h-full rounded-[2rem] border border-border bg-bg-card backdrop-blur-xl p-10 shadow-2xl shadow-cyan-500/10 flex flex-col">
        <div className="mb-8 text-center">
          <h2 className="text-3xl font-semibold text-text-primary">
            {pageTitle}
          </h2>
          <p className="mx-auto mt-3 max-w-[420px] text-sm text-text-secondary">
            {pageDescription}
          </p>
        </div>

        {!disablePasswordLogin && (
          <Form {...form}>
            <form
              className="flex h-full min-h-[420px] flex-col justify-center gap-8 text-text-primary"
              data-testid="auth-form"
              data-active={isActiveFace ? 'true' : undefined}
              onSubmit={form.handleSubmit(onCheck)}
            >
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('emailLabel')}</FormLabel>
                    <FormControl>
                      <Input
                        data-testid="auth-email"
                        placeholder={t('emailPlaceholder')}
                        autoComplete="email"
                        {...field}
                      />
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
                          data-testid="auth-nickname"
                          placeholder={t('nicknamePlaceholder')}
                          autoComplete="username"
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
                      <div className="relative">
                        <Input
                          data-testid="auth-password"
                          type={'password'}
                          placeholder={t('passwordPlaceholder')}
                          autoComplete={
                            title === 'login'
                              ? 'current-password'
                              : 'new-password'
                          }
                          {...field}
                        />
                      </div>
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
                          <FormLabel
                            className={cn(' hover:text-text-primary', {
                              'text-text-disabled': !field.value,
                              'text-text-primary': field.value,
                            })}
                          >
                            {t('rememberMe')}
                          </FormLabel>
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              <ButtonLoading
                data-testid="auth-submit"
                type="submit"
                loading={loading}
                className="bg-[var(--button-primary)] hover:bg-[var(--button-primary-hover)] w-full my-8"
              >
                {title === 'login' ? t('login') : t('continue')}
              </ButtonLoading>
            </form>
          </Form>
        )}

        {title === 'login' && channels && channels.length > 0 && (
          <div className={disablePasswordLogin ? 'py-8' : 'mt-3 border'}>
            {channels.map((item) => (
              <Button
                variant={'transparent'}
                key={item.channel}
                onClick={() => handleLoginWithChannel(item.channel)}
                style={{ marginTop: 10 }}
                className={disablePasswordLogin ? 'w-full' : ''}
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

        {!disablePasswordLogin && title === 'login' && registerEnabled && (
          <div className="mt-10 text-center">
            <p className="text-text-disabled text-sm ">
              {t('signInTip')}
              <Button
                data-testid="auth-toggle-register"
                variant={'static'}
                onClick={changeTitle}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium"
              >
                {t('signUp')}
              </Button>
            </p>
          </div>
        )}
        {!disablePasswordLogin && title === 'register' && (
          <div className="mt-10 text-center">
            <p className="text-text-disabled text-sm">
              {t('signUpTip')}
              <Button
                data-testid="auth-toggle-login"
                variant={'static'}
                onClick={changeTitle}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium"
              >
                {t('login')}
              </Button>
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

type CardCompanyProps = {
  title: string;
  par1: string;
  par2: string;
};

function CardCompany({ title, par1, par2 }: CardCompanyProps) {
  return (
    <div className="hidden w-full max-w-[540px] flex-1 min-h-[720px] rounded-[2rem] border border-border bg-bg-card p-8 text-text-primary shadow-2xl shadow-cyan-500/10 backdrop-blur-xl lg:flex lg:flex-col lg:justify-between lg:gap-10">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="flex h-20 w-20 items-center justify-center rounded-[2rem] border border-white/10 bg-white/10 shadow-lg shadow-cyan-500/10">
            <ThemeLogo className="h-14 w-14" />
          </div>
          <div>
            <div className="text-2xl font-semibold text-text-primary">
              MetaGross-AI
            </div>
            <div className="text-sm text-text-secondary">
              AI workspace for teams
            </div>
          </div>
        </div>
      </div>

      <div className="flex flex-1 flex-col justify-center gap-6 text-center lg:text-left">
        <h2 className="text-3xl font-semibold text-text-primary">{title}</h2>
        <div className="space-y-4 text-sm leading-7 text-text-secondary">
          <p>{par1}</p>
          <p>{par2}</p>
        </div>
      </div>

      <div className="rounded-[2rem] border border-border-button bg-bg-component p-6 text-sm text-text-secondary shadow-inner shadow-cyan-500/5 bg-[--note-info-bg] border-[--note-info-border]">
        <p className="font-medium text-text-primary text-[--note-info-title]">
          Build faster with AI-first workflows.
        </p>
        <p className="mt-3 text-sm leading-6 text-text-secondary text-[--note-info-text]">
          Launch agents, share results, and stay in sync with a workspace
          designed for smart teams.
        </p>
      </div>
    </div>
  );
}

const Login = () => {
  const [title, setTitle] = useState('login');
  const navigate = useNavigate();
  const { login, loading: signLoading } = useLogin();
  const { register, loading: registerLoading } = useRegister();
  const { channels, loading: channelsLoading } = useLoginChannels();
  const { login: loginWithChannel, loading: loginWithChannelLoading } =
    useLoginWithChannel();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const [isLoginPage, setIsLoginPage] = useState(true);

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
    setIsLoginPage(title !== 'login');
    if (title === 'login' && !registerEnabled) {
      return;
    }

    setTimeout(() => {
      setTitle(title === 'login' ? 'register' : 'login');
    }, 200);
  };

  const FormSchema = z
    .object({
      nickname: z.string(),
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
  type FormValues = z.infer<typeof FormSchema>;
  const form = useForm<FormValues>({
    defaultValues: {
      nickname: '',
      email: '',
      password: '',
      remember: false,
    },
    resolver: zodResolver(FormSchema),
  });

  const onCheck = async (params: FormValues) => {
    try {
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
    <>
      <Spotlight opcity={0.4} coverage={60} color={'rgb(128, 255, 248)'} />

      <div className="relative min-h-screen overflow-clip login-page">
        <div className="relative z-10 mx-auto flex min-h-screen max-w-[1300px] flex-col justify-center px-4 py-8 sm:px-6 lg:flex-row lg:items-center lg:space-x-12">
          <CardCompany
            title={'Web access for AI-powered workflows'}
            par1={
              'Use the MetaGross-AI web portal to launch agents, explore workflows, and manage your AI workspace from any browser.'
            }
            par2={
              'Trusted performance, fast collaboration, and a clean interface for everything your team builds online.'
            }
          />

          <div className="w-full max-w-[540px] flex-1 h-[720px] flex items-center justify-center">
            <div className="w-full h-full max-w-[540px] flex items-center justify-center">
              <FlipCard3D isLoginPage={isLoginPage}>
                <LoginFormContent
                  isLoginPage={isLoginPage}
                  title={title}
                  form={form}
                  loading={loading}
                  onCheck={onCheck}
                  changeTitle={changeTitle}
                  registerEnabled={registerEnabled}
                  channels={channels || []}
                  handleLoginWithChannel={handleLoginWithChannel}
                  t={t}
                  disablePasswordLogin={!!config?.disablePasswordLogin}
                />
              </FlipCard3D>
            </div>
          </div>
        </div>
        <div className="absolute bottom-6 left-1/2 z-20 flex -translate-x-1/2 items-center justify-center rounded-full bg-bg-card/90   p-2  ">
          <ThemeSwitch />
        </div>
      </div>
    </>
  );
};

export default Login;

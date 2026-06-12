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

  return (
    <div className="flex flex-col items-center justify-center w-full ">
      {/* <div className="text-center mb-8">
         <h2 className="text-xl font-semibold text-text-primary">
          {title === 'login' ? t('loginTitle') : t('signUpTitle')}
        </h2> 
      </div> */}
      <div className=" w-full max-w-[540px] bg-bg-component backdrop-blur-sm rounded-2xl shadow-xl pt-14 pl-10 pr-10 pb-2 border border-border-button ">
        {!disablePasswordLogin && (
          <Form {...form}>
            <form
              className="flex flex-col gap-8 text-text-primary "
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
                className="bg-metallic-gradient border-b-[#00BEB4] border-b-2 hover:bg-metallic-gradient hover:border-b-[#02bcdd] w-full my-8"
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
          <div className="mt-10 text-right">
            <p className="text-text-disabled text-sm">
              {t('signInTip')}
              <Button
                data-testid="auth-toggle-register"
                variant={'transparent'}
                onClick={changeTitle}
                className="text-accent-primary/90 hover:text-accent-primary hover:bg-transparent font-medium border-none transition-colors duration-200"
              >
                {t('signUp')}
              </Button>
            </p>
          </div>
        )}
        {!disablePasswordLogin && title === 'register' && (
          <div className="mt-10 text-right">
            <p className="text-text-disabled text-sm">
              {t('signUpTip')}
              <Button
                data-testid="auth-toggle-login"
                variant={'transparent'}
                onClick={changeTitle}
                className="text-accent-primary/90 hover:text-accent-primary hover:bg-transparent font-medium border-none transition-colors duration-200"
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

      <div className="relative min-h-screen overflow-hidden">
        <div className="relative z-10 mx-auto flex min-h-screen max-w-[1300px] flex-col justify-center px-4 py-8 sm:px-6 lg:flex-row lg:items-center lg:space-x-12">
          <div className="hidden w-full max-w-[540px] flex-1 min-h-[640px] flex-col justify-center rounded-[2rem] border border-white/10 bg-white/5 p-8 text-text-primary shadow-2xl shadow-cyan-500/10 backdrop-blur-xl lg:flex">
            <div className="flex items-center gap-4 mb-8">
              <div className="flex h-20 w-20 items-center justify-center rounded-3xl bg-cyan-500/10 shadow-lg shadow-cyan-500/10">
                <img
                  src="/logo.png"
                  alt="MetaGross-AI logo"
                  className="h-14 w-14"
                />
              </div>
              <div>
                <div className="text-2xl font-semibold">MetaGross-AI</div>
              </div>
            </div>
            <h2 className="text-3xl font-semibold text-text-primary">
              Web access for AI-powered workflows
            </h2>
            <p className="mt-4 text-sm leading-7 text-text-secondary">
              Use the MetaGross-AI web portal to launch agents, explore
              workflows, and manage your AI workspace from any browser.
            </p>
            <p className="mt-6 text-sm leading-7 text-text-secondary">
              Trusted performance, fast collaboration, and a clean interface for
              everything your team builds online.
            </p>
          </div>

          <div className="w-full max-w-[540px] flex-1 min-h-[640px]">
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
    </>
  );
};

export default Login;

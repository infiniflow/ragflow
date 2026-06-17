import SvgIcon from '@/components/svg-icon';
import { useIsDarkTheme } from '@/components/theme-provider';
import { useAuth } from '@/hooks/auth-hooks';
import {
  useLogin,
  useLoginChannels,
  useLoginWithChannel,
  useRegister,
} from '@/hooks/use-login-request';
import { useSystemConfig } from '@/hooks/use-system-request';
import { rsaPsw } from '@/utils';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

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
import HCaptcha from '@hcaptcha/react-hcaptcha';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, UseFormReturn } from 'react-hook-form';
import { z } from 'zod';
import './index.less';
// ─── Left Panel (full background image) ───────────────────────────────────────

function LeftPanel() {
  return (
    <div
      className="hidden lg:flex lg:flex-col lg:justify-between relative w-1/2 min-h-screen overflow-hidden"
      style={{
        backgroundImage: "url('bgImage.png')", // ← replace with your image path
        backgroundSize: 'cover',
        backgroundPosition: 'center',
      }}
    >
      {/* Dark overlay for readability */}
      <div className="absolute inset-0 bg-black/50" />

      {/* Content on top of image */}
      <div className="relative z-10 flex flex-col h-full p-12">
        {/* Logo */}
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-white/10 border border-white/20">
            <ThemeLogo className="h-7 w-7" />
          </div>
          <span className="text-xl font-semibold text-white">MetaGross-AI</span>
        </div>

        {/* Tagline */}
        <div className="mt-auto space-y-4 pb-8">
          <h2 className="text-4xl font-semibold text-white leading-tight">
            Web access for AI-powered workflows
          </h2>
          <p className="text-base text-white/70 leading-relaxed max-w-sm">
            Launch agents, explore workflows, and manage your AI workspace from
            any browser. Trusted performance for everything your team builds.
          </p>
        </div>
      </div>
    </div>
  );
}

// ─── Form ─────────────────────────────────────────────────────────────────────

type AuthFormProps = {
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
  requireCaptcha: boolean;
  captchaKey: string | undefined;
  captchaRef: React.RefObject<HCaptcha>;
};

function AuthForm({
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
  requireCaptcha,
  captchaKey,
  captchaRef,
}: AuthFormProps) {
  const isLogin = title === 'login';
  const navigate = useNavigate();
  const pageTitle = isLogin
    ? 'Sign in to MetaGross-AI'
    : 'Create your MetaGross-AI account';
  const pageDescription = isLogin
    ? 'Use your credentials to access the AI workspace and manage your workflows.'
    : 'Join MetaGross-AI to start using agents, automations, and smart dashboards.';

  const isDark = useIsDarkTheme();

  return (
    <div className="flex flex-col w-full max-w-[460px]">
      {/* Header */}
      <div className="mb-8">
        <h2 className="text-3xl font-semibold text-text-primary">
          {pageTitle}
        </h2>
        <p className="mt-2 text-sm text-text-secondary">{pageDescription}</p>
      </div>

      {/* Password-based form */}
      {!disablePasswordLogin && (
        <Form {...form}>
          <form
            className="flex flex-col gap-5"
            data-testid="auth-form"
            onSubmit={form.handleSubmit(onCheck)}
          >
            {/* Email */}
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

            {/* Nickname — register only */}
            {!isLogin && (
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

            {/* Password */}
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('passwordLabel')}</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="auth-password"
                      type="password"
                      placeholder={t('passwordPlaceholder')}
                      autoComplete={
                        isLogin ? 'current-password' : 'new-password'
                      }
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Forgot password + Remember me row — login only */}
            {isLogin && (
              <div className="flex items-center justify-between -mt-1">
                <FormField
                  control={form.control}
                  name="remember"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <div className="flex gap-2 items-center">
                          <Checkbox
                            checked={field.value}
                            onCheckedChange={(checked) =>
                              field.onChange(checked)
                            }
                          />
                          <FormLabel
                            className={cn('cursor-pointer', {
                              'text-text-disabled': !field.value,
                              'text-text-primary': field.value,
                            })}
                          >
                            {t('rememberMe')}
                          </FormLabel>
                        </div>
                      </FormControl>
                    </FormItem>
                  )}
                />
                <Button
                  variant="static"
                  type="button"
                  onClick={() => navigate('/forgot-password')}
                  className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 text-sm font-medium p-0"
                >
                  {t('forget')}
                </Button>
              </div>
            )}

            {requireCaptcha && (
              <div className="flex justify-left">
                <FormField
                  control={form.control}
                  name="hcaptcha_token"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <HCaptcha
                          key={captchaKey}
                          ref={captchaRef}
                          theme={isDark ? 'dark' : 'light'}
                          sitekey="bcf819c2-a604-4008-9c6a-a37e84cec112"
                          onVerify={(token) => field.onChange(token)}
                          onExpire={() => field.onChange('')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}

            {/* Submit */}
            <ButtonLoading
              data-testid="auth-submit"
              type="submit"
              loading={loading}
              className="bg-[var(--button-primary)] hover:bg-[var(--button-primary-hover)] w-full mt-2"
            >
              {isLogin ? t('login') : t('continue')}
            </ButtonLoading>
          </form>
        </Form>
      )}

      {/* SSO channels — login only */}
      {isLogin && channels && channels.length > 0 && (
        <div
          className={cn('mt-4', {
            'py-4': disablePasswordLogin,
            'border-t mt-6 pt-4': !disablePasswordLogin,
          })}
        >
          {channels.map((item) => (
            <Button
              variant="transparent"
              key={item.channel}
              onClick={() => handleLoginWithChannel(item.channel)}
              className={cn('mt-2', { 'w-full': disablePasswordLogin })}
            >
              <div className="flex items-center gap-2">
                <SvgIcon name={item.icon || 'sso'} width={20} height={20} />
                Sign in with {item.display_name}
              </div>
            </Button>
          ))}
        </div>
      )}

      {/* Switch between login / register */}
      {!disablePasswordLogin && (
        <p className="mt-8 text-center text-sm text-text-secondary">
          {isLogin ? (
            <>
              {t('signInTip')}
              {registerEnabled && (
                <Button
                  data-testid="auth-toggle-register"
                  variant="static"
                  onClick={changeTitle}
                  className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium ml-1"
                >
                  {t('signUp')}
                </Button>
              )}
            </>
          ) : (
            <>
              {t('signUpTip')}
              <Button
                data-testid="auth-toggle-login"
                variant="static"
                onClick={changeTitle}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium ml-1"
              >
                {t('login')}
              </Button>
            </>
          )}
        </p>
      )}
    </div>
  );
}

// ─── Main Page ─────────────────────────────────────────────────────────────────

const Login = () => {
  const [title, setTitle] = useState('login');
  const navigate = useNavigate();
  const { login, loading: signLoading } = useLogin();
  const { register, loading: registerLoading } = useRegister();
  const { channels, loading: channelsLoading } = useLoginChannels();
  const { login: loginWithChannel, loading: loginWithChannelLoading } =
    useLoginWithChannel();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });

  const [requireCaptcha, setRequireCaptcha] = useState(true);
  const [captchaKey, setCaptchaKey] = useState<string>(() =>
    crypto.randomUUID(),
  );
  const captchaRef = useRef<HCaptcha>(null);

  const loading =
    signLoading ||
    registerLoading ||
    channelsLoading ||
    loginWithChannelLoading;
  const { config } = useSystemConfig();
  const registerEnabled = config?.registerEnabled !== 0;

  const { isLogin: isLoggedIn } = useAuth();
  useEffect(() => {
    if (isLoggedIn) navigate('/');
  }, [isLoggedIn, navigate]);

  const handleLoginWithChannel = async (channel: string) => {
    await loginWithChannel(channel);
  };

  const changeTitle = () => {
    if (title === 'login' && !registerEnabled) return;
    setTitle(title === 'login' ? 'register' : 'login');
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
      hcaptcha_token: z.string().optional(),
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
    defaultValues: { nickname: '', email: '', password: '', remember: false },
    resolver: zodResolver(FormSchema),
  });

  const onCheck = async (params: FormValues) => {
    if (requireCaptcha && !params.hcaptcha_token) return;
    try {
      const rsaPassWord = rsaPsw(params.password) as string;
      if (title === 'login') {
        const code = await login({
          email: `${params.email}`.trim(),
          password: rsaPassWord,
          hcaptcha_token: params.hcaptcha_token || '',
        });
        if (code === 0) navigate('/');
        // If your API signals captcha_required via an error code or flag:
        if (code === 'captcha_required') {
          setRequireCaptcha(true);
          setCaptchaKey(crypto.randomUUID());
        }
      } else {
        const code = await register({
          nickname: params.nickname,
          email: params.email,
          password: rsaPassWord,
        });
        if (code === 0) setTitle('login');
      }
    } catch (errorInfo: any) {
      setCaptchaKey(crypto.randomUUID());
      if (errorInfo?.captcha_required) setRequireCaptcha(true);
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <div className="flex min-h-screen bg-bg-card">
      {/* Left: full background image */}
      <LeftPanel />

      {/* Right: form panel */}
      <div className="flex flex-1 flex-col items-center justify-center px-8 py-12 relative">
        {/* Theme switch top-right */}
        <div className="absolute top-6 right-6">
          <ThemeSwitch />
        </div>

        <AuthForm
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
          requireCaptcha={requireCaptcha}
          captchaKey={captchaKey}
          captchaRef={captchaRef}
        />
      </div>
    </div>
  );
};

export default Login;

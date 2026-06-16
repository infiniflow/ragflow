import ThemeLogo from '@/components/theme-logo';
import ThemeSwitch from '@/components/theme-switch';
import { Button, ButtonLoading } from '@/components/ui/button';
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
import { ArrowLeft, CheckCircle, XCircle } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { z } from 'zod';

// ─── Left Panel (reused from login page) ──────────────────────────────────────

function LeftPanel() {
  return (
    <div
      className="hidden lg:flex lg:flex-col lg:justify-between relative w-1/2 min-h-screen overflow-hidden"
      style={{
        backgroundImage: "url('bgImage.png')",
        backgroundSize: 'cover',
        backgroundPosition: 'center',
      }}
    >
      <div className="absolute inset-0 bg-black/50" />

      <div className="relative z-10 flex flex-col h-full p-12">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-white/10 border border-white/20">
            <ThemeLogo className="h-7 w-7" />
          </div>
          <span className="text-xl font-semibold text-white">MetaGross-AI</span>
        </div>

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

// ─── Forgot Password Form ──────────────────────────────────────────────────────

const ForgotPassword = () => {
  const navigate = useNavigate();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const [status, setStatus] = useState<'form' | 'success' | 'error'>('form');
  const [loading, setLoading] = useState(false);

  const FormSchema = z.object({
    email: z.string().email({ message: 'Please enter a valid email address.' }),
  });

  type FormValues = z.infer<typeof FormSchema>;

  const form = useForm<FormValues>({
    defaultValues: { email: '' },
    resolver: zodResolver(FormSchema),
  });

  const onSubmit = async (values: FormValues) => {
    setLoading(true);
    try {
      // TODO: call your reset password API here
      // await sendResetEmail(values.email);
      console.log('Reset email sent to:', values.email);
      setStatus('success');
    } catch (err) {
      console.error(err);
      setStatus('error');
    } finally {
      setLoading(false);
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

        <div className="flex flex-col w-full max-w-[460px]">

          {/* ── Success state ── */}
          {status === 'success' && (
            <div className="flex flex-col items-center text-center gap-6">
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-green-500/10 border border-green-500/20">
                <CheckCircle className="w-8 h-8 text-green-500" />
              </div>
              <div>
                <h2 className="text-3xl font-semibold text-text-primary">
                  Email sent!
                </h2>
                <p className="mt-2 text-sm text-text-secondary">
                  Check your inbox. A verification code has been sent to your email.
                  Didn't receive it? Check your spam folder or try again.
                </p>
              </div>
              <Button
                variant="static"
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium"
                onClick={() => setStatus('form')}
              >
                Resend email
              </Button>
              <Button
                variant="static"
                onClick={() => navigate('/login')}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 text-sm font-medium flex items-center gap-1"
              >
                <ArrowLeft size={14} />
                Back to Sign in
              </Button>
            </div>
          )}

          {/* ── Error state ── */}
          {status === 'error' && (
            <div className="flex flex-col items-center text-center gap-6">
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-red-500/10 border border-red-500/20">
                <XCircle className="w-8 h-8 text-red-500" />
              </div>
              <div>
                <h2 className="text-3xl font-semibold text-text-primary">
                  Something went wrong
                </h2>
                <p className="mt-2 text-sm text-text-secondary">
                  We couldn't send a reset email. Please try again.
                </p>
              </div>
              <ButtonLoading
                loading={false}
                className="bg-[var(--button-primary)] hover:bg-[var(--button-primary-hover)] w-full"
                onClick={() => setStatus('form')}
              >
                Try again
              </ButtonLoading>
              <Button
                variant="static"
                onClick={() => navigate('/login')}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 text-sm font-medium flex items-center gap-1"
              >
                <ArrowLeft size={14} />
                Back to Sign in
              </Button>
            </div>
          )}

          {/* ── Form state ── */}
          {status === 'form' && (
            <>
              <div className="mb-8">
                <h2 className="text-3xl font-semibold text-text-primary">
                  Reset your password
                </h2>
                <p className="mt-2 text-sm text-text-secondary">
                  Enter your email and we'll send you a code to reset your password.
                </p>
              </div>

              <Form {...form}>
                <form
                  className="flex flex-col gap-5"
                  onSubmit={form.handleSubmit(onSubmit)}
                >
                  <FormField
                    control={form.control}
                    name="email"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel required>Email</FormLabel>
                        <FormControl>
                          <Input
                            placeholder="Please input email"
                            autoComplete="email"
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <ButtonLoading
                    type="submit"
                    loading={loading}
                    className="bg-[var(--button-primary)] hover:bg-[var(--button-primary-hover)] w-full mt-2"
                  >
                    Send
                  </ButtonLoading>
                </form>
              </Form>

              <div className="mt-8 text-center">
                <Button
                  variant="static"
                  onClick={() => navigate('/login')}
                  className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 text-sm font-medium flex items-center gap-1 mx-auto"
                >
                  <ArrowLeft size={14} />
                  Back to Sign in
                </Button>
              </div>
            </>
          )}

        </div>
      </div>
    </div>
  );
};

export default ForgotPassword;

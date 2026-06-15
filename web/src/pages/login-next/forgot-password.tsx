// forgot-password.tsx
import Spotlight from '@/components/spotlight';
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
import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { z } from 'zod';

const ForgotPassword = () => {
  const navigate = useNavigate();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const [sent, setSent] = useState(false);
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
      setSent(true);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <Spotlight opcity={0.4} coverage={60} color={'rgb(128, 255, 248)'} />

      <div className="relative min-h-screen overflow-clip login-page">
        <div className="relative z-10 mx-auto flex min-h-screen max-w-[1300px] items-center justify-center px-4 py-8">
          
          <div className="w-full max-w-[540px] rounded-[2rem] border border-border bg-bg-card backdrop-blur-xl p-10 shadow-2xl shadow-cyan-500/10 flex flex-col gap-8">
            
            {/* Header */}
            <div className="text-center">
              <h2 className="text-3xl font-semibold text-text-primary">
                Reset your password
              </h2>
              <p className="mx-auto mt-3 max-w-[420px] text-sm text-text-secondary">
                {sent
                  ? 'Check your inbox. A verification code has been sent to your email.'
                  : "Enter your email and we'll send you a code to reset your password."}
              </p>
            </div>

            {/* Form or Success state */}
            {!sent ? (
              <Form {...form}>
                <form
                  className="flex flex-col gap-6"
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
            ) : (
              // Success state
              <div className="rounded-[1.5rem] border border-border-button bg-bg-component p-6 text-sm text-text-secondary shadow-inner shadow-cyan-500/5 bg-[--note-info-bg] border-[--note-info-border] text-center">
                <p className="font-medium text-text-primary text-[--note-info-title]">
                  Email sent!
                </p>
                <p className="mt-2 text-sm leading-6 text-[--note-info-text]">
                  Didn't receive it? Check your spam folder or try again.
                </p>
                <Button
                  variant="static"
                  className="mt-4 !text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 font-medium"
                  onClick={() => setSent(false)}
                >
                  Resend email
                </Button>
              </div>
            )}

            {/* Back to login */}
            <div className="text-center">
              <Button
                variant="static"
                onClick={() => navigate('/login')}
                className="!text-[var(--accent-primary)] hover:!text-[var(--accent-primary)]/90 text-sm font-medium flex items-center gap-1 mx-auto"
              >
                <ArrowLeft size={14} />
                Back to Sign in
              </Button>
            </div>

          </div>
        </div>

        {/* Theme switcher */}
        <div className="absolute bottom-6 left-1/2 z-20 flex -translate-x-1/2 items-center justify-center rounded-full bg-bg-card/90 p-2">
          <ThemeSwitch />
        </div>
      </div>
    </>
  );
};

export default ForgotPassword;
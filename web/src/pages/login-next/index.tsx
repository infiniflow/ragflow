import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useTranslate } from '@/hooks/common-hooks';
import { SignUpForm, VerifyEmailForm } from './form';

export function SignUpCard() {
  const { t } = useTranslate('login');

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>{t('login')}</CardTitle>
      </CardHeader>
      <CardContent>
        <SignUpForm></SignUpForm>
      </CardContent>
    </Card>
  );
}

export function VerifyEmailCard() {
  // const { t } = useTranslate('login');

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>Verify email</CardTitle>
      </CardHeader>
      <CardContent>
        <section className="flex gap-y-6 flex-col">
          <div className="flex items-center space-x-4">
            <div className="flex-1 space-y-1">
              <p className="text-sm font-medium leading-none">
                We’ve sent a 6-digit code to
              </p>
              <p className="text-sm text-blue-500">yifanwu92@gmail.com.</p>
            </div>
            <Button>Resend</Button>
          </div>
          <VerifyEmailForm></VerifyEmailForm>
        </section>
      </CardContent>
    </Card>
  );
}

const Login = () => {
  return (
    <>
      <SignUpCard></SignUpCard>
      <VerifyEmailCard></VerifyEmailCard>
    </>
  );
};

export default Login;

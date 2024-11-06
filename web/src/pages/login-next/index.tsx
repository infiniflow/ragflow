import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { useTranslate } from '@/hooks/common-hooks';
import { DiscordLogoIcon, GitHubLogoIcon } from '@radix-ui/react-icons';
import { SignInForm, SignUpForm, VerifyEmailForm } from './form';

function LoginFooter() {
  return (
    <section className="pt-[30px]">
      <Separator />
      <p className="text-center pt-[20px]">or continue with</p>
      <div className="flex gap-4 justify-center pt-[20px]">
        <GitHubLogoIcon className="w-8 h-8"></GitHubLogoIcon>
        <DiscordLogoIcon className="w-8 h-8"></DiscordLogoIcon>
      </div>
    </section>
  );
}

export function SignUpCard() {
  const { t } = useTranslate('login');

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>{t('signUp')}</CardTitle>
      </CardHeader>
      <CardContent>
        <SignUpForm></SignUpForm>
        <LoginFooter></LoginFooter>
      </CardContent>
    </Card>
  );
}

export function SignInCard() {
  const { t } = useTranslate('login');

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>{t('login')}</CardTitle>
      </CardHeader>
      <CardContent>
        <SignInForm></SignInForm>
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
      <SignInCard></SignInCard>
      <VerifyEmailCard></VerifyEmailCard>
    </>
  );
};

export default Login;

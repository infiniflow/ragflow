import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { useTranslate } from '@/hooks/common-hooks';
import { DiscordLogoIcon, GitHubLogoIcon } from '@radix-ui/react-icons';
import { useSearchParams } from 'umi';
import { SignInForm, SignUpForm, VerifyEmailForm } from './form';
import { Step, useSwitchStep } from './hooks';

function LoginFooter() {
  return (
    <section className="pt-4">
      <Separator />
      <p className="text-center pt-4">or continue with</p>
      <div className="flex gap-4 justify-center pt-[20px]">
        <GitHubLogoIcon className="w-8 h-8"></GitHubLogoIcon>
        <DiscordLogoIcon className="w-8 h-8"></DiscordLogoIcon>
      </div>
    </section>
  );
}

export function SignUpCard() {
  const { t } = useTranslate('login');

  const { switchStep } = useSwitchStep(Step.SignIn);

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>{t('signUp')}</CardTitle>
      </CardHeader>
      <CardContent>
        <SignUpForm></SignUpForm>
        <div className="text-center">
          <Button variant={'link'} className="pt-6" onClick={switchStep}>
            Already have an account? Log In
          </Button>
        </div>
        <LoginFooter></LoginFooter>
      </CardContent>
    </Card>
  );
}

export function SignInCard() {
  const { t } = useTranslate('login');
  const { switchStep } = useSwitchStep(Step.SignUp);

  return (
    <Card className="w-[400px]">
      <CardHeader>
        <CardTitle>{t('login')}</CardTitle>
      </CardHeader>
      <CardContent>
        <SignInForm></SignInForm>
        <Button
          className="w-full mt-2"
          onClick={switchStep}
          variant={'secondary'}
        >
          {t('signUp')}
        </Button>
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
  const [searchParams] = useSearchParams();
  const step = Number((searchParams.get('step') ?? Step.SignIn) as Step);

  return (
    <div className="w-full h-full flex items-center pl-[15%] bg-[url('@/assets/svg/next-login-bg.svg')] bg-cover bg-center">
      <div className="inline-block bg-colors-background-neutral-standard rounded-lg">
        {step === Step.SignIn && <SignInCard></SignInCard>}
        {step === Step.SignUp && <SignUpCard></SignUpCard>}
        {step === Step.VerifyEmail && <VerifyEmailCard></VerifyEmailCard>}
      </div>
    </div>
  );
};

export default Login;

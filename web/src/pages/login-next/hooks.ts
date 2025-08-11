import { useCallback } from 'react';
import { useSearchParams } from 'umi';

export enum Step {
  SignIn,
  SignUp,
  ForgotPassword,
  ResetPassword,
  VerifyEmail,
}

export const useSwitchStep = (step: Step) => {
  const [_, setSearchParams] = useSearchParams();
  console.log('🚀 ~ useSwitchStep ~ _:', _);
  const switchStep = useCallback(() => {
    setSearchParams(new URLSearchParams({ step: step.toString() }));
  }, [setSearchParams, step]);

  return { switchStep };
};

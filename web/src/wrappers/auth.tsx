import { useAuth } from '@/hooks/auth-hooks';
import { LoginType, redirectToLogin } from '@/utils/authorization-util';
import { Outlet } from 'umi';

export default () => {
  const { isLogin, error } = useAuth();

  if (isLogin === true) {
    return <Outlet />;
  } else if (isLogin === false) {
    if (error) {
      return redirectToLogin({ error });
    }

    return redirectToLogin({ type: LoginType.AUTO });
  }

  return <></>;
};

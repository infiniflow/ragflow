import { useAuth } from '@/hooks/auth-hooks';
import { redirectToLogin } from '@/utils/authorization-util';
import { Outlet } from 'umi';

export default () => {
  const { isLogin } = useAuth();
  if (isLogin === true) {
    return <Outlet />;
  } else if (isLogin === false) {
    redirectToLogin();
  }

  return <></>;
};

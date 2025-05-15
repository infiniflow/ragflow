import { useAuth } from '@/hooks/auth-hooks';
import { redirectToLogin } from '@/utils/authorization-util';
import { useEffect } from 'react';
import { Outlet } from 'umi';

export default () => {
  const { isLogin, error } = useAuth();

  useEffect(() => {
    const autoLoginCallback = sessionStorage.getItem('auto_login_callback');
    if (isLogin && autoLoginCallback) {
      if (autoLoginCallback !== window.location.href) {
        window.location.href = autoLoginCallback;
      }
      sessionStorage.removeItem('auto_login_callback');
    }
  }, [isLogin]);

  if (isLogin === true) {
    return <Outlet />;
  } else if (isLogin === false) {
    if (error) {
      return redirectToLogin({ error });
    }

    return redirectToLogin();
  }

  return <></>;
};

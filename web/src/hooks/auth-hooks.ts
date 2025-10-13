import message from '@/components/ui/message';
import authorizationUtil from '@/utils/authorization-util';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import { useAutoLogin } from './auto-login-hooks';

export const useOAuthCallback = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const error = currentQueryParameters.get('error');
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );
  const navigate = useNavigate();

  useEffect(() => {
    if (error) {
      message.error(error);
      setTimeout(() => {
        navigate('/login');
        newQueryParameters.delete('error');
        setSearchParams(newQueryParameters);
      }, 1000);
      return;
    }

    const auth = currentQueryParameters.get('auth');
    if (auth) {
      authorizationUtil.setAuthorization(auth);
      newQueryParameters.delete('auth');
      setSearchParams(newQueryParameters);
      navigate('/');
    }
  }, [
    error,
    currentQueryParameters,
    newQueryParameters,
    navigate,
    setSearchParams,
  ]);

  console.debug(currentQueryParameters.get('auth'));
  return currentQueryParameters.get('auth');
};

export const useAuth = () => {
  const auth = useOAuthCallback();
  const [isLogin, setIsLogin] = useState<Nullable<boolean>>(null);
  const { autoLogin } = useAutoLogin();
  const autoLoginAttempted = useRef(false);

  useEffect(() => {
    const checkAuth = async () => {
      const hasAuth = !!authorizationUtil.getAuthorization() || !!auth;

      if (!hasAuth && !autoLoginAttempted.current) {
        // 尝试自动登录
        autoLoginAttempted.current = true;
        try {
          const loginSuccess = await autoLogin();
          setIsLogin(loginSuccess);
        } catch (error) {
          console.error('Auto login failed:', error);
          setIsLogin(false);
        }
      } else {
        setIsLogin(hasAuth);
      }
    };

    checkAuth();
  }, [auth, autoLogin]);

  return { isLogin };
};

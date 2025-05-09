import authorizationUtil from '@/utils/authorization-util';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'umi';

export const useLoginWithGithub = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const error = currentQueryParameters.get('error');
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );

  const auth = currentQueryParameters.get('auth');

  useEffect(() => {
    if (auth) {
      authorizationUtil.setAuthorization(auth);
    }
  }, [auth]);

  const authResult = useMemo(() => {
    return {
      auth,
      error,
    };
  }, [auth, error]);

  return authResult;
};

export const useAuth = () => {
  const { auth, error } = useLoginWithGithub();
  const [isLogin, setIsLogin] = useState<Nullable<boolean>>(null);

  useEffect(() => {
    setIsLogin(!!authorizationUtil.getAuthorization() || !!auth);
  }, [auth]);

  return { isLogin, error };
};

import authorizationUtil from '@/utils/authorization-util';
import { useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';

export const useLoginWithGithub = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const error = currentQueryParameters.get('error');
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  if (error) {
    message.error(error);
    navigate('/login');
    newQueryParameters.delete('error');
    setSearchParams(newQueryParameters);
    return;
  }

  const auth = currentQueryParameters.get('auth');

  if (auth) {
    authorizationUtil.setAuthorization(auth);
    newQueryParameters.delete('auth');
    setSearchParams(newQueryParameters);
    queryClient.invalidateQueries({ queryKey: ['tenantInfo'] });
  }
  return auth;
};

export const useAuth = () => {
  const auth = useLoginWithGithub();
  const [isLogin, setIsLogin] = useState<Nullable<boolean>>(null);

  useEffect(() => {
    setIsLogin(!!authorizationUtil.getAuthorization() || !!auth);
  }, [auth]);

  return { isLogin };
};

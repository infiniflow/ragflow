import authorizationUtil from '@/utils/authorizationUtil';
import { useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';

export const useAuth = () => {
  const [isLogin, setIsLogin] = useState(
    () => !!authorizationUtil.getAuthorization(),
  );

  return { isLogin };
};

export const useLoginWithGithub = () => {
  const [currentQueryParameters, setSearchParams] = useSearchParams();
  const error = currentQueryParameters.get('error');
  const newQueryParameters: URLSearchParams = useMemo(
    () => new URLSearchParams(currentQueryParameters.toString()),
    [currentQueryParameters],
  );
  const navigate = useNavigate();

  if (error) {
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
  }
};

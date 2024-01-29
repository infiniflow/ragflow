import authorizationUtil from '@/utils/authorizationUtil';
import { useState } from 'react';

export const useAuth = () => {
  const [isLogin, setIsLogin] = useState(
    () => !!authorizationUtil.getAuthorization(),
  );

  return { isLogin };
};

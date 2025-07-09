import { useLogout } from '@/hooks/login-hooks';
import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();
  const { logout } = useLogout();

  const handleMenuClick = useCallback(
    (key: Routes) => () => {
      if (key === Routes.Logout) {
        logout();
      } else {
        navigate(`${Routes.ProfileSetting}${key}`);
      }
    },
    [logout, navigate],
  );

  return { handleMenuClick };
};

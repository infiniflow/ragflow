import {
  ProfileSettingBaseKey,
  ProfileSettingRouteKey,
} from '@/constants/setting';
import { useLogout } from '@/hooks/login-hooks';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();
  const { logout } = useLogout();

  const handleMenuClick = useCallback(
    (key: ProfileSettingRouteKey) => () => {
      if (key === ProfileSettingRouteKey.Logout) {
        logout();
      } else {
        navigate(`/${ProfileSettingBaseKey}/${key}`);
      }
    },
    [logout, navigate],
  );

  return { handleMenuClick };
};

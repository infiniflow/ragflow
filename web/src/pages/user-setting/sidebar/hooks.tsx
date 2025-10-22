import { useLogout } from '@/hooks/login-hooks';
import { Routes } from '@/routes';
import { useCallback, useState } from 'react';
import { useNavigate } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();
  const [active, setActive] = useState<Routes>();
  const { logout } = useLogout();

  const handleMenuClick = useCallback(
    (key: Routes) => () => {
      if (key === Routes.Logout) {
        logout();
      } else {
        setActive(key);
        navigate(`${Routes.UserSetting}${key}`);
      }
    },
    [logout, navigate],
  );

  return { handleMenuClick, active };
};

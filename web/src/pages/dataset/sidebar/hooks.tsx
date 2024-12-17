import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();

  const handleMenuClick = useCallback(
    (key: Routes) => () => {
      navigate(`${Routes.DatasetBase}${key}`);
    },
    [navigate],
  );

  return { handleMenuClick };
};

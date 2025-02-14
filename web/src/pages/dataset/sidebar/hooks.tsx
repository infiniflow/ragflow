import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useNavigate, useParams } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();
  const { id } = useParams();

  const handleMenuClick = useCallback(
    (key: Routes) => () => {
      navigate(`${Routes.DatasetBase}${key}/${id}`);
    },
    [id, navigate],
  );

  return { handleMenuClick };
};

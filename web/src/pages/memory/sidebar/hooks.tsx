import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useParams } from 'react-router';

export const useHandleMenuClick = () => {
  const { navigateToMemorySection } = useNavigatePage();
  const { id } = useParams();

  const handleMenuClick = useCallback(
    (key: Routes) => navigateToMemorySection(id, key),
    [id, navigateToMemorySection],
  );

  return { handleMenuClick };
};

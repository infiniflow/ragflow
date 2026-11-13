import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useParams } from 'react-router';

export const useHandleMenuClick = () => {
  const { navigateToDatasetSection } = useNavigatePage();
  const { id } = useParams();

  const handleMenuClick = useCallback(
    (key: Routes, data?: any) => navigateToDatasetSection(id, key, data),
    [id, navigateToDatasetSection],
  );

  return { handleMenuClick };
};

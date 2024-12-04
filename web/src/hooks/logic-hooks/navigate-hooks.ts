import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useNavigatePage = () => {
  const navigate = useNavigate();

  const navigateToDatasetList = useCallback(() => {
    navigate(Routes.Datasets);
  }, [navigate]);

  const navigateToDataset = useCallback(() => {
    navigate(Routes.Dataset);
  }, [navigate]);

  const navigateToHome = useCallback(() => {
    navigate(Routes.Home);
  }, [navigate]);

  return { navigateToDatasetList, navigateToDataset, navigateToHome };
};

import { DatasetBaseKey, KnowledgeRouteKey } from '@/constants/knowledge';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useHandleMenuClick = () => {
  const navigate = useNavigate();

  const handleMenuClick = useCallback(
    (key: KnowledgeRouteKey) => () => {
      navigate(`/${DatasetBaseKey}/${key}`);
    },
    [navigate],
  );

  return { handleMenuClick };
};

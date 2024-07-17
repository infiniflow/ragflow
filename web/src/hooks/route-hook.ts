import {
  KnowledgeRouteKey,
  KnowledgeSearchParams,
} from '@/constants/knowledge';
import { useCallback } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'umi';

export enum SegmentIndex {
  Second = '2',
  Third = '3',
}

export const useSegmentedPathName = (index: SegmentIndex) => {
  const { pathname } = useLocation();

  const pathArray = pathname.split('/');
  return pathArray[index] || '';
};

export const useSecondPathName = () => {
  return useSegmentedPathName(SegmentIndex.Second);
};

export const useThirdPathName = () => {
  return useSegmentedPathName(SegmentIndex.Third);
};

export const useGetKnowledgeSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    documentId:
      currentQueryParameters.get(KnowledgeSearchParams.DocumentId) || '',
    knowledgeId:
      currentQueryParameters.get(KnowledgeSearchParams.KnowledgeId) || '',
  };
};

export const useNavigateWithFromState = () => {
  const navigate = useNavigate();
  return useCallback(
    (path: string) => {
      navigate(path, { state: { from: path } });
    },
    [navigate],
  );
};

export const useNavigateToDataset = () => {
  const navigate = useNavigate();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  return useCallback(() => {
    navigate(`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeId}`);
  }, [knowledgeId, navigate]);
};

import {
  KnowledgeRouteKey,
  KnowledgeSearchParams,
} from '@/constants/knowledge';
import { useCallback } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router';

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
    type: currentQueryParameters.get(KnowledgeSearchParams.Type) || '',
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

export const useGetPaginationParams = () => {
  const [currentQueryParameters] = useSearchParams();

  return {
    page: currentQueryParameters.get('page') || 1,
    size: currentQueryParameters.get('size') || 10,
  };
};

export const useSetPaginationParams = () => {
  const [queryParameters, setSearchParams] = useSearchParams();
  // const newQueryParameters: URLSearchParams = useMemo(
  //   () => new URLSearchParams(queryParameters.toString()),
  //   [queryParameters],
  // );

  const setPaginationParams = useCallback(
    (page: number = 1, pageSize?: number) => {
      queryParameters.set('page', page.toString());
      if (pageSize) {
        queryParameters.set('size', pageSize.toString());
      }
      setSearchParams(queryParameters);
    },
    [setSearchParams, queryParameters],
  );

  return {
    setPaginationParams,
    page: Number(queryParameters.get('page')) || 1,
    size: Number(queryParameters.get('size')) || 50,
  };
};

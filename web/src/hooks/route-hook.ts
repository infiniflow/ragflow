/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  KnowledgeRouteKey,
  KnowledgeSearchParams,
} from '@/constants/knowledge';
import { Routes } from '@/routes';
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
  const { pathname } = useLocation();
  const isDataflowResultPage = pathname === Routes.DataflowResult;

  return {
    type: currentQueryParameters.get(KnowledgeSearchParams.Type) || '',
    documentId:
      currentQueryParameters.get(KnowledgeSearchParams.DocumentId) || '',
    knowledgeId: isDataflowResultPage
      ? currentQueryParameters.get('knowledgeId') || ''
      : currentQueryParameters.get(KnowledgeSearchParams.KnowledgeId) || '',
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

const PageSizeStorageKeyPrefix = 'RAGFlowPageSize:';

const getStoredPageSize = (pathname: string) =>
  Number(localStorage.getItem(`${PageSizeStorageKeyPrefix}${pathname}`));

export const useSetPaginationParams = () => {
  const [queryParameters, setSearchParams] = useSearchParams();
  const { pathname } = useLocation();

  const setPaginationParams = useCallback(
    (page: number = 1, pageSize?: number) => {
      queryParameters.set('page', page.toString());
      if (pageSize) {
        queryParameters.set('size', pageSize.toString());
        // Persist the page size per main page so it survives navigation
        // to other pages, where the URL search params are dropped.
        localStorage.setItem(
          `${PageSizeStorageKeyPrefix}${pathname}`,
          pageSize.toString(),
        );
      }
      setSearchParams(queryParameters);
    },
    [setSearchParams, queryParameters, pathname],
  );

  return {
    setPaginationParams,
    page: Number(queryParameters.get('page')) || 1,
    size:
      Number(queryParameters.get('size')) || getStoredPageSize(pathname) || 50,
  };
};

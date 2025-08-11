// src/pages/next-searches/hooks.ts

import searchService from '@/services/search-service';
import { useMutation, useQuery } from '@tanstack/react-query';
import { message } from 'antd';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';

interface CreateSearchProps {
  name: string;
  description?: string;
}

interface CreateSearchResponse {
  id: string;
  name: string;
  description: string;
}

export const useCreateSearch = () => {
  const { t } = useTranslation();

  const {
    data,
    isLoading,
    isError,
    mutateAsync: createSearchMutation,
  } = useMutation<CreateSearchResponse, Error, CreateSearchProps>({
    mutationKey: ['createSearch'],
    mutationFn: async (props) => {
      const { data: response } = await searchService.createSearch(props);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to create search');
      }
      return response.data;
    },
    onSuccess: () => {
      message.success(t('message.created'));
    },
    onError: (error) => {
      message.error(t('message.error', { error: error.message }));
    },
  });

  const createSearch = useCallback(
    (props: CreateSearchProps) => {
      return createSearchMutation(props);
    },
    [createSearchMutation],
  );

  return { data, isLoading, isError, createSearch };
};

export interface SearchListParams {
  keywords?: string;
  parser_id?: string;
  page?: number;
  page_size?: number;
  orderby?: string;
  desc?: boolean;
  owner_ids?: string;
}
export interface ISearchAppProps {
  avatar: any;
  create_time: number;
  created_by: string;
  description: string;
  id: string;
  name: string;
  nickname: string;
  status: string;
  tenant_avatar: any;
  tenant_id: string;
  update_time: number;
}
interface SearchListResponse {
  code: number;
  data: {
    search_apps: Array<ISearchAppProps>;
    total: number;
  };
  message: string;
}

export const useFetchSearchList = (params?: SearchListParams) => {
  const [searchParams, setSearchParams] = useState<SearchListParams>({
    page: 1,
    page_size: 10,
    ...params,
  });

  const { data, isLoading, isError } = useQuery<SearchListResponse, Error>({
    queryKey: ['searchList', searchParams],
    queryFn: async () => {
      const { data: response } =
        await searchService.getSearchList(searchParams);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch search list');
      }
      return response;
    },
  });

  const setSearchListParams = (newParams: SearchListParams) => {
    setSearchParams((prevParams) => ({
      ...prevParams,
      ...newParams,
    }));
  };

  return { data, isLoading, isError, searchParams, setSearchListParams };
};

interface DeleteSearchProps {
  search_id: string;
}

interface DeleteSearchResponse {
  code: number;
  data: boolean;
  message: string;
}

export const useDeleteSearch = () => {
  const { t } = useTranslation();

  const {
    data,
    isLoading,
    isError,
    mutateAsync: deleteSearchMutation,
  } = useMutation<DeleteSearchResponse, Error, DeleteSearchProps>({
    mutationKey: ['deleteSearch'],
    mutationFn: async (props) => {
      const response = await searchService.deleteSearch(props);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to delete search');
      }
      return response;
    },
    onSuccess: () => {
      message.success(t('message.deleted'));
    },
    onError: (error) => {
      message.error(t('message.error', { error: error.message }));
    },
  });

  const deleteSearch = useCallback(
    (props: DeleteSearchProps) => {
      return deleteSearchMutation(props);
    },
    [deleteSearchMutation],
  );

  return { data, isLoading, isError, deleteSearch };
};

export interface ISearchAppDetailProps {
  avatar: any;
  created_by: string;
  description: string;
  id: string;
  name: string;
  search_config: {
    cross_languages: string[];
    doc_ids: string[];
    highlight: boolean;
    kb_ids: string[];
    keyword: boolean;
    query_mindmap: boolean;
    related_search: boolean;
    rerank_id: string;
    similarity_threshold: number;
    summary: boolean;
    top_k: number;
    use_kg: boolean;
    vector_similarity_weight: number;
    web_search: boolean;
  };
  tenant_id: string;
  update_time: number;
}

interface SearchDetailResponse {
  code: number;
  data: ISearchAppDetailProps;
  message: string;
}

export const useFetchSearchDetail = () => {
  const { id } = useParams();
  const { data, isLoading, isError } = useQuery<SearchDetailResponse, Error>({
    queryKey: ['searchDetail', id],
    queryFn: async () => {
      const { data: response } = await searchService.getSearchDetail({
        search_id: id,
      });
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch search detail');
      }
      return response;
    },
  });

  return { data: data?.data, isLoading, isError };
};

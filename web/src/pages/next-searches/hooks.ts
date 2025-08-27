// src/pages/next-searches/hooks.ts

import message from '@/components/ui/message';
import searchService from '@/services/search-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'umi';
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

  return { data, isError, createSearch };
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

  const { data, isLoading, isError, refetch } = useQuery<
    SearchListResponse,
    Error
  >({
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

  return {
    data,
    isLoading,
    isError,
    searchParams,
    setSearchListParams,
    refetch,
  };
};

interface DeleteSearchProps {
  search_id: string;
}

interface DeleteSearchResponse {
  code: number;
  data: boolean;
  message: string;
}

export interface IllmSettingProps {
  llm_id: string;
  parameter: string;
  temperature?: number;
  top_p?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
}
interface IllmSettingEnableProps {
  temperatureEnabled?: boolean;
  topPEnabled?: boolean;
  presencePenaltyEnabled?: boolean;
  frequencyPenaltyEnabled?: boolean;
}
export interface ISearchAppDetailProps {
  avatar: any;
  created_by: string;
  description: string;
  id: string;
  name: string;
  search_config: {
    cross_languages: string[];
    doc_ids: string[];
    chat_id: string;
    highlight: boolean;
    kb_ids: string[];
    keyword: boolean;
    query_mindmap: boolean;
    related_search: boolean;
    rerank_id: string;
    use_rerank?: boolean;
    similarity_threshold: number;
    summary: boolean;
    llm_setting: IllmSettingProps & IllmSettingEnableProps;
    top_k: number;
    use_kg: boolean;
    vector_similarity_weight: number;
    web_search: boolean;
    chat_settingcross_languages: string[];
    meta_data_filter?: {
      method: string;
      manual: { key: string; op: string; value: string }[];
    };
  };
  tenant_id: string;
  update_time: number;
}

interface SearchDetailResponse {
  code: number;
  data: ISearchAppDetailProps;
  message: string;
}

export const useFetchSearchDetail = (tenantId?: string) => {
  const { id } = useParams();

  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const searchId = id || shared_id;
  let param: { search_id: string | null; tenant_id?: string } = {
    search_id: searchId,
  };
  if (shared_id) {
    param = {
      search_id: searchId,
      tenant_id: tenantId,
    };
  }
  const fetchSearchDetailFunc = shared_id
    ? searchService.getSearchDetailShare
    : searchService.getSearchDetail;

  const { data, isLoading, isError } = useQuery<SearchDetailResponse, Error>({
    queryKey: ['searchDetail', searchId],
    enabled: !shared_id || !!tenantId,
    queryFn: async () => {
      const { data: response } = await fetchSearchDetailFunc(param);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to fetch search detail');
      }
      return response;
    },
  });

  return { data: data?.data, isLoading, isError };
};

export const useDeleteSearch = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isError,
    mutateAsync: deleteSearchMutation,
  } = useMutation<DeleteSearchResponse, Error, DeleteSearchProps>({
    mutationKey: ['deleteSearch'],
    mutationFn: async (props) => {
      const { data: response } = await searchService.deleteSearch(props);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to delete search');
      }

      queryClient.invalidateQueries({ queryKey: ['searchList'] });
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

  return { data, isError, deleteSearch };
};

export type IUpdateSearchProps = Omit<ISearchAppDetailProps, 'id'> & {
  search_id: string;
};

export const useUpdateSearch = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isError,
    mutateAsync: updateSearchMutation,
  } = useMutation<any, Error, IUpdateSearchProps>({
    mutationKey: ['updateSearch'],
    mutationFn: async (formData) => {
      const { data: response } =
        await searchService.updateSearchSetting(formData);
      if (response.code !== 0) {
        throw new Error(response.message || 'Failed to update search');
      }
      return response.data;
    },
    onSuccess: (data, variables) => {
      message.success(t('message.updated'));
      queryClient.invalidateQueries({
        queryKey: ['searchDetail', variables.search_id],
      });
    },
    onError: (error) => {
      message.error(t('message.error', { error: error.message }));
    },
  });

  const updateSearch = useCallback(
    (formData: IUpdateSearchProps) => {
      return updateSearchMutation(formData);
    },
    [updateSearchMutation],
  );

  return { data, isError, updateSearch };
};

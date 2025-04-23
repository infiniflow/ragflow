import { IDocumentInfo } from '@/interfaces/database/document';
import i18n from '@/locales/config';
import kbService from '@/services/knowledge-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { message } from 'antd';
import { get } from 'lodash';
import { useCallback } from 'react';
import { useParams } from 'umi';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useGetKnowledgeSearchParams } from './route-hook';

export const enum DocumentApiAction {
  UploadDocument = 'uploadDocument',
  FetchDocumentList = 'fetchDocumentList',
  UpdateDocumentStatus = 'updateDocumentStatus',
  RunDocumentByIds = 'runDocumentByIds',
}

export const useUploadNextDocument = () => {
  const queryClient = useQueryClient();
  const { id } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.UploadDocument],
    mutationFn: async (fileList: File[]) => {
      const formData = new FormData();
      formData.append('kb_id', id!);
      fileList.forEach((file: any) => {
        formData.append('file', file);
      });

      try {
        const ret = await kbService.document_upload(formData);
        const code = get(ret, 'data.code');

        if (code === 0 || code === 500) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });
        }
        return ret?.data;
      } catch (error) {
        console.warn(error);
        return {
          code: 500,
          message: error + '',
        };
      }
    },
  });

  return { uploadDocument: mutateAsync, loading, data };
};

export const useFetchDocumentList = () => {
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const { id } = useParams();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery<{
    docs: IDocumentInfo[];
    total: number;
  }>({
    queryKey: [
      DocumentApiAction.FetchDocumentList,
      debouncedSearchString,
      pagination,
    ],
    initialData: { docs: [], total: 0 },
    refetchInterval: 15000,
    enabled: !!knowledgeId || !!id,
    queryFn: async () => {
      const ret = await kbService.get_document_list({
        kb_id: knowledgeId || id,
        keywords: debouncedSearchString,
        page_size: pagination.pageSize,
        page: pagination.current,
      });
      if (ret.data.code === 0) {
        return ret.data.data;
      }

      return {
        docs: [],
        total: 0,
      };
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      setPagination({ page: 1 });
      handleInputChange(e);
    },
    [handleInputChange, setPagination],
  );

  return {
    loading,
    searchString,
    documents: data.docs,
    pagination: { ...pagination, total: data?.total },
    handleInputChange: onInputChange,
    setPagination,
  };
};

export const useSetDocumentStatus = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.UpdateDocumentStatus],
    mutationFn: async ({
      status,
      documentId,
    }: {
      status: boolean;
      documentId: string;
    }) => {
      const { data } = await kbService.document_change_status({
        doc_id: documentId,
        status: Number(status),
      });
      if (data.code === 0) {
        message.success(i18n.t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
      }
      return data;
    },
  });

  return { setDocumentStatus: mutateAsync, data, loading };
};

export const useRunDocument = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.RunDocumentByIds],
    mutationFn: async ({
      documentIds,
      run,
      shouldDelete,
    }: {
      documentIds: string[];
      run: number;
      shouldDelete: boolean;
    }) => {
      queryClient.invalidateQueries({
        queryKey: [DocumentApiAction.FetchDocumentList],
      });

      const ret = await kbService.document_run({
        doc_ids: documentIds,
        run,
        delete: shouldDelete,
      });
      const code = get(ret, 'data.code');
      if (code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
        message.success(i18n.t('message.operated'));
      }

      return code;
    },
  });

  return { runDocumentByIds: mutateAsync, loading, data };
};

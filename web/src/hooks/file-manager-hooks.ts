import { ResponseType } from '@/interfaces/database/base';
import { IFolder } from '@/interfaces/database/file-manager';
import { IConnectRequestBody } from '@/interfaces/request/file-manager';
import fileManagerService from '@/services/file-manager-service';
import { downloadFileFromBlob } from '@/utils/file-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { PaginationProps, UploadFile, message } from 'antd';
import React, { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'umi';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useSetPaginationParams } from './route-hook';

export const useGetFolderId = () => {
  const [searchParams] = useSearchParams();
  const id = searchParams.get('folderId') as string;

  return id ?? '';
};

export interface IListResult {
  searchString: string;
  handleInputChange: React.ChangeEventHandler<HTMLInputElement>;
  pagination: PaginationProps;
  setPagination: (pagination: { page: number; pageSize: number }) => void;
  loading: boolean;
}

export const useFetchPureFileList = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: ['fetchPureFileList'],
    gcTime: 0,

    mutationFn: async (parentId: string) => {
      const { data } = await fileManagerService.listFile({
        parent_id: parentId,
      });

      return data;
    },
  });

  return { loading, fetchList: mutateAsync };
};

export const useFetchFileList = (): ResponseType<any> & IListResult => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const id = useGetFolderId();

  const { data, isFetching: loading } = useQuery({
    queryKey: [
      'fetchFileList',
      {
        id,
        searchString,
        ...pagination,
      },
    ],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await fileManagerService.listFile({
        parent_id: id,
        keywords: searchString,
        page_size: pagination.pageSize,
        page: pagination.current,
      });

      return data;
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
    ...data,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.data?.total },
    setPagination,
    loading,
  };
};

export const useDeleteFile = () => {
  const { setPaginationParams } = useSetPaginationParams();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteFile'],
    mutationFn: async (params: { fileIds: string[]; parentId: string }) => {
      const { data } = await fileManagerService.removeFile(params);
      if (data.code === 0) {
        setPaginationParams(1); // TODO: There should be a better way to paginate the request list
        queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
      }
      return data.code;
    },
  });

  return { data, loading, deleteFile: mutateAsync };
};

export const useDownloadFile = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['downloadFile'],
    mutationFn: async (params: { id: string; filename?: string }) => {
      const response = await fileManagerService.getFile({}, params.id);
      const blob = new Blob([response.data], { type: response.data.type });
      downloadFileFromBlob(blob, params.filename);
    },
  });
  return { data, loading, downloadFile: mutateAsync };
};

export const useRenameFile = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['renameFile'],
    mutationFn: async (params: { fileId: string; name: string }) => {
      const { data } = await fileManagerService.renameFile(params);
      if (data.code === 0) {
        message.success(t('message.renamed'));
        queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
      }
      return data.code;
    },
  });

  return { data, loading, renameFile: mutateAsync };
};

export const useFetchParentFolderList = (): IFolder[] => {
  const id = useGetFolderId();
  const { data } = useQuery({
    queryKey: ['fetchParentFolderList', id],
    initialData: [],
    enabled: !!id,
    queryFn: async () => {
      const { data } = await fileManagerService.getAllParentFolder({
        fileId: id,
      });

      return data?.data?.parent_folders?.toReversed() ?? [];
    },
  });

  return data;
};

export const useCreateFolder = () => {
  const { setPaginationParams } = useSetPaginationParams();
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createFolder'],
    mutationFn: async (params: { parentId: string; name: string }) => {
      const { data } = await fileManagerService.createFolder({
        ...params,
        type: 'folder',
      });
      if (data.code === 0) {
        message.success(t('message.created'));
        setPaginationParams(1);
        queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
      }
      return data.code;
    },
  });

  return { data, loading, createFolder: mutateAsync };
};

export const useUploadFile = () => {
  const { setPaginationParams } = useSetPaginationParams();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['uploadFile'],
    mutationFn: async (params: {
      fileList: UploadFile[];
      parentId: string;
    }) => {
      const fileList = params.fileList;
      const pathList = params.fileList.map(
        (file) => (file as any).webkitRelativePath,
      );
      const formData = new FormData();
      formData.append('parent_id', params.parentId);
      fileList.forEach((file: any, index: number) => {
        formData.append('file', file);
        formData.append('path', pathList[index]);
      });
      try {
        const ret = await fileManagerService.uploadFile(formData);
        if (ret?.data.code === 0) {
          message.success(t('message.uploaded'));
          setPaginationParams(1);
          queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
        }
        return ret?.data?.code;
      } catch (error) {
        console.log('🚀 ~ useUploadFile ~ error:', error);
      }
    },
  });

  return { data, loading, uploadFile: mutateAsync };
};

export const useConnectToKnowledge = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['connectFileToKnowledge'],
    mutationFn: async (params: IConnectRequestBody) => {
      const { data } = await fileManagerService.connectFileToKnowledge(params);
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
      }
      return data.code;
    },
  });

  return { data, loading, connectFileToKnowledge: mutateAsync };
};

export interface IMoveFileBody {
  src_file_ids: string[];
  dest_file_id: string; // target folder id
}

export const useMoveFile = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['moveFile'],
    mutationFn: async (params: IMoveFileBody) => {
      const { data } = await fileManagerService.moveFile(params);
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({ queryKey: ['fetchFileList'] });
      }
      return data.code;
    },
  });

  return { data, loading, moveFile: mutateAsync };
};

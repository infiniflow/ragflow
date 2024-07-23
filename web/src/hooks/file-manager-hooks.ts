import { ResponseType } from '@/interfaces/database/base';
import {
  IConnectRequestBody,
  IFileListRequestBody,
} from '@/interfaces/request/file-manager';
import fileManagerService from '@/services/file-manager-service';
import { useMutation, useQuery } from '@tanstack/react-query';
import { PaginationProps, UploadFile } from 'antd';
import React, { useCallback } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import { useGetNextPagination, useHandleSearchChange } from './logic-hooks';
import { useSetPaginationParams } from './route-hook';

export const useGetFolderId = () => {
  const [searchParams] = useSearchParams();
  const id = searchParams.get('folderId') as string;

  return id ?? '';
};

export const useFetchFileList = () => {
  const dispatch = useDispatch();

  const fetchFileList = useCallback(
    (payload: IFileListRequestBody) => {
      return dispatch<any>({
        type: 'fileManager/listFile',
        payload,
      });
    },
    [dispatch],
  );

  return fetchFileList;
};

export interface IListResult {
  searchString: string;
  handleInputChange: React.ChangeEventHandler<HTMLInputElement>;
  pagination: PaginationProps;
  setPagination: (pagination: { page: number; pageSize: number }) => void;
}

export const useFetchNextFileList = (): ResponseType<any> & IListResult => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetNextPagination();
  const id = useGetFolderId();

  const { data } = useQuery({
    queryKey: [
      'fetchFileList',
      // pagination.current,
      // id,
      // pagination.pageSize,
      // searchString,
      {
        id,
        searchString,
        ...pagination,
      },
    ],
    initialData: {},
    queryFn: async (params: any) => {
      console.info(params);
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
  };
};

export const useRemoveFile = () => {
  const dispatch = useDispatch();

  const removeFile = useCallback(
    (fileIds: string[], parentId: string) => {
      return dispatch<any>({
        type: 'fileManager/removeFile',
        payload: { fileIds, parentId },
      });
    },
    [dispatch],
  );

  return removeFile;
};

export const useDeleteFile = () => {
  const { setPaginationParams } = useSetPaginationParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteFile'],
    mutationFn: async (params: { fileIds: string[]; parentId: string }) => {
      const { data } = await fileManagerService.removeFile(params);
      if (data.retcode === 0) {
        setPaginationParams(1);
      }
      return data?.data ?? {};
    },
  });

  return { data, loading, deleteFile: mutateAsync };
};

export const useRenameFile = () => {
  const dispatch = useDispatch();

  const renameFile = useCallback(
    (fileId: string, name: string, parentId: string) => {
      return dispatch<any>({
        type: 'fileManager/renameFile',
        payload: { fileId, name, parentId },
      });
    },
    [dispatch],
  );

  return renameFile;
};

export const useFetchParentFolderList = () => {
  const dispatch = useDispatch();

  const fetchParentFolderList = useCallback(
    (fileId: string) => {
      return dispatch<any>({
        type: 'fileManager/getAllParentFolder',
        payload: { fileId },
      });
    },
    [dispatch],
  );

  return fetchParentFolderList;
};

export const useCreateFolder = () => {
  const dispatch = useDispatch();

  const createFolder = useCallback(
    (parentId: string, name: string) => {
      return dispatch<any>({
        type: 'fileManager/createFolder',
        payload: { parentId, name, type: 'folder' },
      });
    },
    [dispatch],
  );

  return createFolder;
};

export const useSelectFileList = () => {
  const fileList = useSelector((state) => state.fileManager.fileList);

  return fileList;
};

export const useSelectParentFolderList = () => {
  const parentFolderList = useSelector(
    (state) => state.fileManager.parentFolderList,
  );
  return parentFolderList.toReversed();
};

export const useUploadFile = () => {
  const dispatch = useDispatch();

  const uploadFile = useCallback(
    (fileList: UploadFile[], parentId: string) => {
      try {
        return dispatch<any>({
          type: 'fileManager/uploadFile',
          payload: {
            file: fileList,
            parentId,
            path: fileList.map((file) => (file as any).webkitRelativePath),
          },
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch],
  );

  return uploadFile;
};

export const useConnectToKnowledge = () => {
  const dispatch = useDispatch();

  const uploadFile = useCallback(
    (payload: IConnectRequestBody) => {
      try {
        return dispatch<any>({
          type: 'fileManager/connectFileToKnowledge',
          payload,
        });
      } catch (errorInfo) {
        console.log('Failed:', errorInfo);
      }
    },
    [dispatch],
  );

  return uploadFile;
};

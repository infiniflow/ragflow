import { IFileListRequestBody } from '@/interfaces/request/file-manager';
import { useCallback } from 'react';
import { useDispatch, useSelector } from 'umi';

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

export const useRemoveFile = () => {
  const dispatch = useDispatch();

  const removeFile = useCallback(
    (fileIds: string[]) => {
      return dispatch<any>({
        type: 'fileManager/removeFile',
        payload: { fileIds },
      });
    },
    [dispatch],
  );

  return removeFile;
};

export const useRenameFile = () => {
  const dispatch = useDispatch();

  const renameFile = useCallback(
    (fileId: string, name: string) => {
      return dispatch<any>({
        type: 'fileManager/renameFile',
        payload: { fileId, name },
      });
    },
    [dispatch],
  );

  return renameFile;
};

export const useSelectFileList = () => {
  const fileList = useSelector((state) => state.fileManager.fileList);

  return fileList;
};

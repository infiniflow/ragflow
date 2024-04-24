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

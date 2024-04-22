import { useCallback } from 'react';
import { useDispatch } from 'umi';

export const useFetchFileList = () => {
  const dispatch = useDispatch();

  const fetchFileList = useCallback(() => {
    return dispatch<any>({
      type: 'fileManager/listFile',
    });
  }, [dispatch]);

  return fetchFileList;
};

export const useRemoveFile = () => {
  const dispatch = useDispatch();

  const removeFile = useCallback(() => {
    return dispatch<any>({
      type: 'fileManager/removeFile',
    });
  }, [dispatch]);

  return removeFile;
};

export const useRenameFile = () => {
  const dispatch = useDispatch();

  const renameFile = useCallback(() => {
    return dispatch<any>({
      type: 'fileManager/renameFile',
    });
  }, [dispatch]);

  return renameFile;
};

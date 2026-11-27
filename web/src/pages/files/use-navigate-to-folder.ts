import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchParentFolderList,
  useGetFolderId,
} from '@/hooks/use-file-request';
import { IFolder } from '@/interfaces/database/file-manager';
import { useCallback } from 'react';

export const useNavigateToOtherFolder = () => {
  const { navigateToFiles } = useNavigatePage();

  const navigateToOtherFolder = useCallback(
    (folderId: string) => {
      navigateToFiles(folderId);
    },
    [navigateToFiles],
  );

  return navigateToOtherFolder;
};

export const useSelectBreadcrumbItems = () => {
  const parentFolderList = useFetchParentFolderList();

  return parentFolderList.length === 1
    ? []
    : parentFolderList.map((x) => ({
        title: x.name === '/' ? 'root' : x.name,
        folderId: x.id,
      }));
};

export const useCurrentFolder = (rootFolder?: IFolder) => {
  const folderId = useGetFolderId();
  const folderPath = useFetchParentFolderList();

  if (!folderId) {
    return rootFolder;
  }

  const currentFolder = folderPath.at(-1);
  return currentFolder?.id === folderId ? currentFolder : undefined;
};

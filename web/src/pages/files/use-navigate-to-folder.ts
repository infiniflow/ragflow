import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchParentFolderList } from '@/hooks/use-file-request';
import { Routes } from '@/routes';
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
        path: `${Routes.Files}?folderId=${x.id}`,
      }));
};

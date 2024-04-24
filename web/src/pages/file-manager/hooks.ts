import {
  useSetModalState,
  useShowDeleteConfirm,
  useTranslate,
} from '@/hooks/commonHooks';
import {
  useConnectToKnowledge,
  useCreateFolder,
  useFetchFileList,
  useFetchParentFolderList,
  useRemoveFile,
  useRenameFile,
  useSelectFileList,
  useSelectParentFolderList,
  useUploadFile,
} from '@/hooks/fileManagerHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { Pagination } from '@/interfaces/common';
import { IFile } from '@/interfaces/database/file-manager';
import { getFilePathByWebkitRelativePath } from '@/utils/fileUtil';
import { PaginationProps } from 'antd';
import { UploadFile } from 'antd/lib';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useDispatch, useNavigate, useSearchParams, useSelector } from 'umi';

export const useGetFolderId = () => {
  const [searchParams] = useSearchParams();
  const id = searchParams.get('folderId') as string;

  return id;
};

export const useFetchDocumentListOnMount = () => {
  const fetchDocumentList = useFetchFileList();
  const fileList = useSelectFileList();
  const id = useGetFolderId();

  const dispatch = useDispatch();

  useEffect(() => {
    fetchDocumentList({ parent_id: id });
  }, [dispatch, fetchDocumentList, id]);

  return { fetchDocumentList, fileList };
};

export const useGetPagination = (
  fetchDocumentList: (payload: IFile) => any,
) => {
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const { t } = useTranslate('common');

  const setPagination = useCallback(
    (pageNumber = 1, pageSize?: number) => {
      const pagination: Pagination = {
        current: pageNumber,
      } as Pagination;
      if (pageSize) {
        pagination.pageSize = pageSize;
      }
      dispatch({
        type: 'kFModel/setPagination',
        payload: pagination,
      });
    },
    [dispatch],
  );

  const onPageChange: PaginationProps['onChange'] = useCallback(
    (pageNumber: number, pageSize: number) => {
      setPagination(pageNumber, pageSize);
      fetchDocumentList();
    },
    [fetchDocumentList, setPagination],
  );

  const pagination: PaginationProps = useMemo(() => {
    return {
      showQuickJumper: true,
      total: kFModel.total,
      showSizeChanger: true,
      current: kFModel.pagination.current,
      pageSize: kFModel.pagination.pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
      showTotal: (total) => `${t('total')} ${total}`,
    };
  }, [kFModel, onPageChange, t]);

  return {
    pagination,
    setPagination,
    total: kFModel.total,
    searchString: kFModel.searchString,
  };
};

export const useHandleSearchChange = (setPagination: () => void) => {
  const dispatch = useDispatch();

  const throttledGetDocumentList = useCallback(() => {
    dispatch({
      type: 'kFModel/throttledGetDocumentList',
    });
  }, [dispatch]);

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const value = e.target.value;
      dispatch({ type: 'kFModel/setSearchString', payload: value });
      setPagination();
      throttledGetDocumentList();
    },
    [setPagination, throttledGetDocumentList, dispatch],
  );

  return { handleInputChange };
};

export const useGetRowSelection = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

  const rowSelection = {
    selectedRowKeys,
    onChange: (newSelectedRowKeys: React.Key[]) => {
      setSelectedRowKeys(newSelectedRowKeys);
    },
  };

  return rowSelection;
};

export const useNavigateToOtherFolder = () => {
  const navigate = useNavigate();
  const navigateToOtherFolder = useCallback(
    (folderId: string) => {
      navigate(`/file?folderId=${folderId}`);
    },
    [navigate],
  );

  return navigateToOtherFolder;
};

export const useRenameCurrentFile = () => {
  const [file, setFile] = useState<IFile>({} as IFile);
  const {
    visible: fileRenameVisible,
    hideModal: hideFileRenameModal,
    showModal: showFileRenameModal,
  } = useSetModalState();
  const renameFile = useRenameFile();

  const onFileRenameOk = useCallback(
    async (name: string) => {
      const ret = await renameFile(file.id, name, file.parent_id);

      if (ret === 0) {
        hideFileRenameModal();
      }
    },
    [renameFile, file, hideFileRenameModal],
  );

  const loading = useOneNamespaceEffectsLoading('fileManager', ['renameFile']);

  const handleShowFileRenameModal = useCallback(
    async (record: IFile) => {
      setFile(record);
      showFileRenameModal();
    },
    [showFileRenameModal],
  );

  return {
    fileRenameLoading: loading,
    initialFileName: file.name,
    onFileRenameOk,
    fileRenameVisible,
    hideFileRenameModal,
    showFileRenameModal: handleShowFileRenameModal,
  };
};

export const useSelectBreadcrumbItems = () => {
  const parentFolderList = useSelectParentFolderList();
  const id = useGetFolderId();
  const fetchParentFolderList = useFetchParentFolderList();

  useEffect(() => {
    if (id) {
      fetchParentFolderList(id);
    }
  }, [id, fetchParentFolderList]);

  return parentFolderList.length === 1
    ? []
    : parentFolderList.map((x) => ({
        title: x.name === '/' ? 'root' : x.name,
        path: `/file?folderId=${x.id}`,
      }));
};

export const useHandleCreateFolder = () => {
  const {
    visible: folderCreateModalVisible,
    hideModal: hideFolderCreateModal,
    showModal: showFolderCreateModal,
  } = useSetModalState();
  const createFolder = useCreateFolder();
  const id = useGetFolderId();

  const onFolderCreateOk = useCallback(
    async (name: string) => {
      const ret = await createFolder(id, name);

      if (ret === 0) {
        hideFolderCreateModal();
      }
    },
    [createFolder, hideFolderCreateModal, id],
  );

  const loading = useOneNamespaceEffectsLoading('fileManager', [
    'createFolder',
  ]);

  return {
    folderCreateLoading: loading,
    onFolderCreateOk,
    folderCreateModalVisible,
    hideFolderCreateModal,
    showFolderCreateModal,
  };
};

export const useHandleDeleteFile = (fileIds: string[]) => {
  const removeDocument = useRemoveFile();
  const showDeleteConfirm = useShowDeleteConfirm();
  const parentId = useGetFolderId();

  const handleRemoveFile = () => {
    showDeleteConfirm({
      onOk: () => {
        return removeDocument(fileIds, parentId);
      },
    });
  };

  return { handleRemoveFile };
};

export const useSelectFileListLoading = () => {
  return useOneNamespaceEffectsLoading('fileManager', ['listFile']);
};

export const useHandleUploadFile = () => {
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const uploadFile = useUploadFile();
  const id = useGetFolderId();

  const onFileUploadOk = useCallback(
    async (fileList: UploadFile[]) => {
      console.info('fileList', fileList);
      if (fileList.length > 0) {
        const ret = await uploadFile(
          fileList[0],
          id,
          getFilePathByWebkitRelativePath(fileList[0] as any),
        );

        if (ret === 0) {
          hideFileUploadModal();
        }
      }
    },
    [uploadFile, hideFileUploadModal, id],
  );

  const loading = useOneNamespaceEffectsLoading('fileManager', ['uploadFile']);

  return {
    fileUploadLoading: loading,
    onFileUploadOk,
    fileUploadVisible,
    hideFileUploadModal,
    showFileUploadModal,
  };
};

export const useHandleConnectToKnowledge = () => {
  const {
    visible: connectToKnowledgeVisible,
    hideModal: hideConnectToKnowledgeModal,
    showModal: showConnectToKnowledgeModal,
  } = useSetModalState();
  const connectToKnowledge = useConnectToKnowledge();
  const id = useGetFolderId();
  const [fileIds, setFileIds] = useState<string[]>([]);

  const onConnectToKnowledgeOk = useCallback(
    async (knowledgeIds: string[]) => {
      const ret = await connectToKnowledge({
        parentId: id,
        fileIds,
        kbIds: knowledgeIds,
      });

      if (ret === 0) {
        hideConnectToKnowledgeModal();
      }
    },
    [connectToKnowledge, hideConnectToKnowledgeModal, id, fileIds],
  );

  const loading = useOneNamespaceEffectsLoading('fileManager', [
    'connectFileToKnowledge',
  ]);

  const handleShowConnectToKnowledgeModal = useCallback(
    (ids: string[]) => {
      setFileIds(ids);
      showConnectToKnowledgeModal();
    },
    [showConnectToKnowledgeModal],
  );

  return {
    connectToKnowledgeLoading: loading,
    onConnectToKnowledgeOk,
    connectToKnowledgeVisible,
    hideConnectToKnowledgeModal,
    showConnectToKnowledgeModal: handleShowConnectToKnowledgeModal,
  };
};

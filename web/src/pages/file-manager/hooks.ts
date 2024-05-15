import { useSetModalState, useShowDeleteConfirm } from '@/hooks/commonHooks';
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
import { useGetPagination, useSetPagination } from '@/hooks/logicHooks';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { IFile } from '@/interfaces/database/file-manager';
import { PaginationProps } from 'antd';
import { TableRowSelection } from 'antd/es/table/interface';
import { UploadFile } from 'antd/lib';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useDispatch, useNavigate, useSearchParams, useSelector } from 'umi';

export const useGetFolderId = () => {
  const [searchParams] = useSearchParams();
  const id = searchParams.get('folderId') as string;

  return id ?? '';
};

export const useFetchDocumentListOnMount = () => {
  const fetchDocumentList = useFetchFileList();
  const fileList = useSelectFileList();
  const id = useGetFolderId();
  const { searchString, pagination } = useSelector(
    (state) => state.fileManager,
  );
  const { pageSize, current } = pagination;

  const dispatch = useDispatch();

  useEffect(() => {
    fetchDocumentList({
      parent_id: id,
      keywords: searchString,
      page_size: pageSize,
      page: current,
    });
  }, [dispatch, fetchDocumentList, id, current, pageSize, searchString]);

  return { fetchDocumentList, fileList };
};

export const useGetFilesPagination = () => {
  const { pagination } = useSelector((state) => state.fileManager);

  const setPagination = useSetPagination('fileManager');

  const onPageChange: PaginationProps['onChange'] = useCallback(
    (pageNumber: number, pageSize: number) => {
      setPagination(pageNumber, pageSize);
    },
    [setPagination],
  );

  const { pagination: paginationInfo } = useGetPagination(
    pagination.total,
    pagination.current,
    pagination.pageSize,
    onPageChange,
  );

  return {
    pagination: paginationInfo,
    setPagination,
  };
};

export const useHandleSearchChange = () => {
  const dispatch = useDispatch();
  const { searchString } = useSelector((state) => state.fileManager);
  const setPagination = useSetPagination('fileManager');

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const value = e.target.value;
      dispatch({ type: 'fileManager/setSearchString', payload: value });
      setPagination();
    },
    [setPagination, dispatch],
  );

  return { handleInputChange, searchString };
};

export const useGetRowSelection = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

  const rowSelection: TableRowSelection<IFile> = {
    selectedRowKeys,
    getCheckboxProps: (record) => {
      return { disabled: record.source_type === 'knowledgebase' };
    },
    onChange: (newSelectedRowKeys: React.Key[]) => {
      setSelectedRowKeys(newSelectedRowKeys);
    },
  };

  return { rowSelection, setSelectedRowKeys };
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

export const useHandleDeleteFile = (
  fileIds: string[],
  setSelectedRowKeys: (keys: string[]) => void,
) => {
  const removeDocument = useRemoveFile();
  const showDeleteConfirm = useShowDeleteConfirm();
  const parentId = useGetFolderId();

  const handleRemoveFile = () => {
    showDeleteConfirm({
      onOk: async () => {
        const retcode = await removeDocument(fileIds, parentId);
        if (retcode === 0) {
          setSelectedRowKeys([]);
        }
        return;
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
    async (fileList: UploadFile[]): Promise<number | undefined> => {
      if (fileList.length > 0) {
        const ret: number = await uploadFile(fileList, id);
        console.info(ret);
        if (ret === 0) {
          hideFileUploadModal();
        }
        return ret;
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
  const [record, setRecord] = useState<IFile>({} as IFile);

  const initialValue = useMemo(() => {
    return Array.isArray(record?.kbs_info)
      ? record?.kbs_info?.map((x) => x.kb_id)
      : [];
  }, [record?.kbs_info]);

  const onConnectToKnowledgeOk = useCallback(
    async (knowledgeIds: string[]) => {
      const ret = await connectToKnowledge({
        parentId: id,
        fileIds: [record.id],
        kbIds: knowledgeIds,
      });

      if (ret === 0) {
        hideConnectToKnowledgeModal();
      }
      return ret;
    },
    [connectToKnowledge, hideConnectToKnowledgeModal, id, record.id],
  );

  const loading = useOneNamespaceEffectsLoading('fileManager', [
    'connectFileToKnowledge',
  ]);

  const handleShowConnectToKnowledgeModal = useCallback(
    (record: IFile) => {
      setRecord(record);
      showConnectToKnowledgeModal();
    },
    [showConnectToKnowledgeModal],
  );

  return {
    initialValue,
    connectToKnowledgeLoading: loading,
    onConnectToKnowledgeOk,
    connectToKnowledgeVisible,
    hideConnectToKnowledgeModal,
    showConnectToKnowledgeModal: handleShowConnectToKnowledgeModal,
  };
};

export const useHandleBreadcrumbClick = () => {
  const navigate = useNavigate();
  const setPagination = useSetPagination('fileManager');

  const handleBreadcrumbClick = useCallback(
    (path?: string) => {
      if (path) {
        setPagination();
        navigate(path);
      }
    },
    [setPagination, navigate],
  );

  return { handleBreadcrumbClick };
};

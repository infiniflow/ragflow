import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  useConnectToKnowledge,
  useCreateFolder,
  useDeleteFile,
  useFetchParentFolderList,
  useMoveFile,
  useRenameFile,
  useUploadFile,
} from '@/hooks/file-manager-hooks';
import { IFile } from '@/interfaces/database/file-manager';
import kbService from '@/services/knowledge-service';
import { TableRowSelection } from 'antd/es/table/interface';
import { UploadFile, message } from 'antd/lib';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'umi';

export const useGetFolderId = () => {
  const [searchParams] = useSearchParams();
  const id = searchParams.get('folderId') as string;

  return id ?? '';
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
  const { renameFile, loading } = useRenameFile();

  const onFileRenameOk = useCallback(
    async (name: string) => {
      const ret = await renameFile({
        fileId: file.id,
        name,
      });

      if (ret === 0) {
        hideFileRenameModal();
      }
    },
    [renameFile, file, hideFileRenameModal],
  );

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
  const parentFolderList = useFetchParentFolderList();

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
  const { createFolder, loading } = useCreateFolder();
  const id = useGetFolderId();

  const onFolderCreateOk = useCallback(
    async (name: string) => {
      const ret = await createFolder({ parentId: id, name });

      if (ret === 0) {
        hideFolderCreateModal();
      }
    },
    [createFolder, hideFolderCreateModal, id],
  );

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
  const { deleteFile: removeDocument } = useDeleteFile();
  const showDeleteConfirm = useShowDeleteConfirm();
  const parentId = useGetFolderId();

  const handleRemoveFile = () => {
    showDeleteConfirm({
      onOk: async () => {
        const code = await removeDocument({ fileIds, parentId });
        if (code === 0) {
          setSelectedRowKeys([]);
        }
        return;
      },
    });
  };

  return { handleRemoveFile };
};

export const useHandleUploadFile = () => {
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const { uploadFile, loading } = useUploadFile();
  const id = useGetFolderId();
  const { t } = useTranslation();

  const onFileUploadOk = useCallback(
    async (
      fileUploadData:
        | { parseOnCreation: boolean; directoryFileList: UploadFile[] }
        | UploadFile[],
    ): Promise<number | undefined> => {
      let fileList: UploadFile[] = [];
      let parseOnCreation = false;

      if (Array.isArray(fileUploadData)) {
        fileList = fileUploadData;
      } else {
        fileList = fileUploadData.directoryFileList || [];
        parseOnCreation = fileUploadData.parseOnCreation;
      }

      if (fileList.length > 0) {
        try {
          const result = await uploadFile({ fileList, parentId: id });

          // 如果上传成功且需要解析文件
          if (
            result.code === 0 &&
            parseOnCreation &&
            result.files &&
            result.files.length > 0
          ) {
            try {
              // 收集所有文件中的doc_id（来自kb_info）
              const docIds: string[] = [];

              result.files.forEach((file: any) => {
                // 检查kb_info是否存在且非空
                if (file.kb_info && file.kb_info.length > 0) {
                  // 从kb_info中获取doc_id
                  file.kb_info.forEach((info: any) => {
                    if (info.doc_id) {
                      docIds.push(info.doc_id);
                    }
                  });
                }
              });

              // 如果有doc_id，则调用document/run接口解析文档
              if (docIds.length > 0) {
                message.info(t('message.parsing'));

                // 使用kbService替代直接fetch调用
                const runResult = await kbService.document_run({
                  doc_ids: docIds,
                  run: 1,
                  delete: false,
                });

                if (runResult.data.code === 0) {
                  message.success(t('message.parseSuccess'));
                } else {
                  message.error(t('message.parseFailed'));
                }
              } else if (parseOnCreation) {
                message.info(t('message.noFilesToParse'));
              }
            } catch (error) {
              console.error('解析文档失败:', error);
              message.error(t('message.parseFailed'));
            }
          }

          // 无论是否解析成功，只要上传成功就关闭弹窗
          if (result.code === 0) {
            message.success(t('message.uploaded'));
            hideFileUploadModal();
          } else {
            message.error(t('message.uploadFailed'));
          }

          return result.code;
        } catch (error) {
          console.error('文件上传错误:', error);
          message.error(t('message.uploadFailed'));
          // 发生错误时也关闭弹窗
          hideFileUploadModal();
          return 500;
        }
      }

      // 如果没有文件，也关闭弹窗
      hideFileUploadModal();
      return 0;
    },
    [uploadFile, hideFileUploadModal, id, t],
  );

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
  const { connectFileToKnowledge: connectToKnowledge, loading } =
    useConnectToKnowledge();
  const [record, setRecord] = useState<IFile>({} as IFile);

  const initialValue = useMemo(() => {
    return Array.isArray(record?.kbs_info)
      ? record?.kbs_info?.map((x) => x.kb_id)
      : [];
  }, [record?.kbs_info]);

  const onConnectToKnowledgeOk = useCallback(
    async (knowledgeIds: string[]) => {
      const ret = await connectToKnowledge({
        fileIds: [record.id],
        kbIds: knowledgeIds,
      });

      if (ret === 0) {
        hideConnectToKnowledgeModal();
      }
      return ret;
    },
    [connectToKnowledge, hideConnectToKnowledgeModal, record.id],
  );

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

  const handleBreadcrumbClick = useCallback(
    (path?: string) => {
      if (path) {
        navigate(path);
      }
    },
    [navigate],
  );

  return { handleBreadcrumbClick };
};

export const useHandleMoveFile = (
  setSelectedRowKeys: (keys: string[]) => void,
) => {
  const {
    visible: moveFileVisible,
    hideModal: hideMoveFileModal,
    showModal: showMoveFileModal,
  } = useSetModalState();
  const { moveFile, loading } = useMoveFile();
  const [sourceFileIds, setSourceFileIds] = useState<string[]>([]);

  const onMoveFileOk = useCallback(
    async (targetFolderId: string) => {
      const ret = await moveFile({
        src_file_ids: sourceFileIds,
        dest_file_id: targetFolderId,
      });

      if (ret === 0) {
        setSelectedRowKeys([]);
        hideMoveFileModal();
      }
      return ret;
    },
    [moveFile, hideMoveFileModal, sourceFileIds, setSelectedRowKeys],
  );

  const handleShowMoveFileModal = useCallback(
    (ids: string[]) => {
      setSourceFileIds(ids);
      showMoveFileModal();
    },
    [showMoveFileModal],
  );

  return {
    initialValue: '',
    moveFileLoading: loading,
    onMoveFileOk,
    moveFileVisible,
    hideMoveFileModal,
    showMoveFileModal: handleShowMoveFileModal,
  };
};

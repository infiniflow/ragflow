import { useSetModalState } from '@/hooks/common-hooks';
import { useConnectToKnowledge, useRenameFile } from '@/hooks/use-file-request';
import { useSelectKnowledgeOptions } from '@/hooks/use-knowledge-request';
import { TableRowSelection } from '@/interfaces/antd-compat';
import { IFile } from '@/interfaces/database/file-manager';
import { ConnectFileToKnowledgeMode } from '@/interfaces/request/file-manager';
import { useCallback, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';

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

export type UseRenameCurrentFileReturnType = ReturnType<
  typeof useRenameCurrentFile
>;

export const useHandleConnectToKnowledge = () => {
  const {
    visible: connectToKnowledgeVisible,
    hideModal: hideConnectToKnowledgeModal,
    showModal: showConnectToKnowledgeModal,
  } = useSetModalState();
  const { connectFileToKnowledge: connectToKnowledge, loading } =
    useConnectToKnowledge();
  const [record, setRecord] = useState<IFile>({} as IFile);
  const [documentIds, setDocumentIds] = useState<string[]>([]);
  const [mode, setMode] = useState<ConnectFileToKnowledgeMode>('replace');
  const knowledgeOptions = useSelectKnowledgeOptions();

  const initialValue = useMemo(() => {
    return Array.isArray(record?.kbs_info)
      ? record?.kbs_info?.map((x) => x.kb_id)
      : [];
  }, [record?.kbs_info]);

  const knowledgeNameMap = useMemo(() => {
    return new Map(
      knowledgeOptions?.map((option) => [
        option.value,
        typeof option.label === 'string' ? option.label : String(option.label),
      ]) ?? [],
    );
  }, [knowledgeOptions]);

  const onConnectToKnowledgeOk = useCallback(
    async (knowledgeIds: string[]) => {
      const ret = await connectToKnowledge({
        fileIds: documentIds,
        kbIds: knowledgeIds,
        mode,
        kbsInfo: knowledgeIds.map((id) => ({
          kb_id: id,
          kb_name: knowledgeNameMap.get(id) ?? id,
        })),
      });

      if (ret === 0) {
        hideConnectToKnowledgeModal();
      }
      return ret;
    },
    [
      connectToKnowledge,
      hideConnectToKnowledgeModal,
      documentIds,
      mode,
      knowledgeNameMap,
    ],
  );

  const handleShowConnectToKnowledgeModal = useCallback(
    (documents: IFile | string[]) => {
      if (Array.isArray(documents)) {
        setDocumentIds(documents);
        setRecord({} as IFile);
        setMode('add');
      } else {
        setRecord(documents);
        setDocumentIds([documents.id]);
        setMode('replace');
      }

      showConnectToKnowledgeModal();
    },
    [showConnectToKnowledgeModal],
  );

  return {
    initialConnectedIds: initialValue,
    connectToKnowledgeLoading: loading,
    onConnectToKnowledgeOk,
    connectToKnowledgeVisible,
    hideConnectToKnowledgeModal,
    showConnectToKnowledgeModal: handleShowConnectToKnowledgeModal,
  };
};

export type UseHandleConnectToKnowledgeReturnType = ReturnType<
  typeof useHandleConnectToKnowledge
>;

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

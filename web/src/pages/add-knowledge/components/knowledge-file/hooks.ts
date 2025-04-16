import { useSetModalState } from '@/hooks/common-hooks';
import {
  useCreateNextDocument,
  useNextWebCrawl,
  useRunNextDocument,
  useSaveNextDocumentName,
  useSetDocumentMeta,
  useSetNextDocumentParser,
  useUploadNextDocument,
} from '@/hooks/document-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { UploadFile } from 'antd';
import { TableRowSelection } from 'antd/es/table/interface';
import { useCallback, useState } from 'react';
import { useNavigate } from 'umi';
import { KnowledgeRouteKey } from './constant';

export const useNavigateToOtherPage = () => {
  const navigate = useNavigate();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const linkToUploadPage = useCallback(() => {
    navigate(`/knowledge/dataset/upload?id=${knowledgeId}`);
  }, [navigate, knowledgeId]);

  const toChunk = useCallback(
    (id: string) => {
      navigate(
        `/knowledge/${KnowledgeRouteKey.Dataset}/chunk?id=${knowledgeId}&doc_id=${id}`,
      );
    },
    [navigate, knowledgeId],
  );

  return { linkToUploadPage, toChunk };
};

export const useRenameDocument = (documentId: string) => {
  const { saveName, loading } = useSaveNextDocumentName();

  const {
    visible: renameVisible,
    hideModal: hideRenameModal,
    showModal: showRenameModal,
  } = useSetModalState();

  const onRenameOk = useCallback(
    async (name: string) => {
      const ret = await saveName({ documentId, name });
      if (ret === 0) {
        hideRenameModal();
      }
    },
    [hideRenameModal, saveName, documentId],
  );

  return {
    renameLoading: loading,
    onRenameOk,
    renameVisible,
    hideRenameModal,
    showRenameModal,
  };
};

export const useCreateEmptyDocument = () => {
  const { createDocument, loading } = useCreateNextDocument();

  const {
    visible: createVisible,
    hideModal: hideCreateModal,
    showModal: showCreateModal,
  } = useSetModalState();

  const onCreateOk = useCallback(
    async (name: string) => {
      const ret = await createDocument(name);
      if (ret === 0) {
        hideCreateModal();
      }
    },
    [hideCreateModal, createDocument],
  );

  return {
    createLoading: loading,
    onCreateOk,
    createVisible,
    hideCreateModal,
    showCreateModal,
  };
};

export const useChangeDocumentParser = (documentId: string) => {
  const { setDocumentParser, loading } = useSetNextDocumentParser();

  const {
    visible: changeParserVisible,
    hideModal: hideChangeParserModal,
    showModal: showChangeParserModal,
  } = useSetModalState();

  const onChangeParserOk = useCallback(
    async (parserId: string, parserConfig: IChangeParserConfigRequestBody) => {
      const ret = await setDocumentParser({
        parserId,
        documentId,
        parserConfig,
      });
      if (ret === 0) {
        hideChangeParserModal();
      }
    },
    [hideChangeParserModal, setDocumentParser, documentId],
  );

  return {
    changeParserLoading: loading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal,
  };
};

export const useGetRowSelection = () => {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

  const rowSelection: TableRowSelection<IDocumentInfo> = {
    selectedRowKeys,
    onChange: (newSelectedRowKeys: React.Key[]) => {
      setSelectedRowKeys(newSelectedRowKeys);
    },
  };

  return rowSelection;
};

export const useHandleUploadDocument = () => {
  const {
    visible: documentUploadVisible,
    hideModal: hideDocumentUploadModal,
    showModal: showDocumentUploadModal,
  } = useSetModalState();
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [uploadProgress, setUploadProgress] = useState<number>(0);
  const { uploadDocument, loading } = useUploadNextDocument();
  const { runDocumentByIds, loading: _ } = useRunNextDocument();

  const onDocumentUploadOk = useCallback(
    async ({
      parseOnCreation,
      directoryFileList,
    }: {
      directoryFileList: UploadFile[];
      parseOnCreation: boolean;
    }): Promise<number | undefined> => {
      const processFileGroup = async (filesPart: UploadFile[]) => {
        // set status to uploading on files
        setFileList(
          fileList.map((file) => {
            if (!filesPart.includes(file)) {
              return file;
            }

            let newFile = file;
            newFile.status = 'uploading';
            newFile.percent = 1;
            return newFile;
          }),
        );

        const ret = await uploadDocument(filesPart);

        const files = ret?.data || [];
        const succesfulFilenames = files.map((file: any) => file.name);

        // set status to done or error on files (based on response)
        setFileList(
          fileList.map((file) => {
            if (!filesPart.includes(file)) {
              return file;
            }

            let newFile = file;
            newFile.status = succesfulFilenames.includes(file.name)
              ? 'done'
              : 'error';
            newFile.percent = 100;
            newFile.response = ret.message;
            return newFile;
          }),
        );

        return {
          code: ret?.code,
          fileIds: files.map((file: any) => file.id),
          totalSuccess: succesfulFilenames.length,
        };
      };
      const totalFiles = fileList.length;

      if (directoryFileList.length > 0) {
        const ret = await uploadDocument(directoryFileList);
        if (ret?.code === 0) {
          hideDocumentUploadModal();
        }
        if (totalFiles === 0) {
          return 0;
        }
      }

      if (totalFiles === 0) {
        console.log('No files to upload');
        hideDocumentUploadModal();
        return 0;
      }

      let totalSuccess = 0;
      let codes = [];
      let toRunFileIds: any[] = [];
      for (let i = 0; i < totalFiles; i += 10) {
        setUploadProgress(Math.floor((i / totalFiles) * 100));
        const files = fileList.slice(i, i + 10);
        const {
          code,
          totalSuccess: count,
          fileIds,
        } = await processFileGroup(files);
        codes.push(code);
        totalSuccess += count;
        toRunFileIds = toRunFileIds.concat(fileIds);
      }

      const allSuccess = codes.every((code) => code === 0);
      const any500 = codes.some((code) => code === 500);

      let code = 500;
      if (allSuccess || (any500 && totalSuccess === totalFiles)) {
        code = 0;
        hideDocumentUploadModal();
      }

      if (parseOnCreation) {
        await runDocumentByIds({
          documentIds: toRunFileIds,
          run: 1,
          shouldDelete: false,
        });
      }

      setUploadProgress(100);

      return code;
    },
    [fileList, uploadDocument, hideDocumentUploadModal, runDocumentByIds],
  );

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    uploadFileList: fileList,
    setUploadFileList: setFileList,
    uploadProgress,
    setUploadProgress,
  };
};

export const useHandleWebCrawl = () => {
  const {
    visible: webCrawlUploadVisible,
    hideModal: hideWebCrawlUploadModal,
    showModal: showWebCrawlUploadModal,
  } = useSetModalState();
  const { webCrawl, loading } = useNextWebCrawl();

  const onWebCrawlUploadOk = useCallback(
    async (name: string, url: string) => {
      const ret = await webCrawl({ name, url });
      if (ret === 0) {
        hideWebCrawlUploadModal();
        return 0;
      }
      return -1;
    },
    [webCrawl, hideWebCrawlUploadModal],
  );

  return {
    webCrawlUploadLoading: loading,
    onWebCrawlUploadOk,
    webCrawlUploadVisible,
    hideWebCrawlUploadModal,
    showWebCrawlUploadModal,
  };
};

export const useHandleRunDocumentByIds = (id: string) => {
  const { runDocumentByIds, loading } = useRunNextDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;

  const handleRunDocumentByIds = async (
    documentId: string,
    isRunning: boolean,
    shouldDelete: boolean = false,
  ) => {
    if (isLoading) {
      return;
    }
    setCurrentId(documentId);
    try {
      await runDocumentByIds({
        documentIds: [documentId],
        run: isRunning ? 2 : 1,
        shouldDelete,
      });
      setCurrentId('');
    } catch (error) {
      setCurrentId('');
    }
  };

  return {
    handleRunDocumentByIds,
    loading: isLoading,
  };
};

export const useShowMetaModal = (documentId: string) => {
  const { setDocumentMeta, loading } = useSetDocumentMeta();

  const {
    visible: setMetaVisible,
    hideModal: hideSetMetaModal,
    showModal: showSetMetaModal,
  } = useSetModalState();

  const onSetMetaModalOk = useCallback(
    async (meta: string) => {
      const ret = await setDocumentMeta({
        documentId,
        meta,
      });
      if (ret === 0) {
        hideSetMetaModal();
      }
    },
    [setDocumentMeta, documentId, hideSetMetaModal],
  );

  return {
    setMetaLoading: loading,
    onSetMetaModalOk,
    setMetaVisible,
    hideSetMetaModal,
    showSetMetaModal,
  };
};

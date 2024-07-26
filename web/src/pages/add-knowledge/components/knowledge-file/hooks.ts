import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import {
  useCreateDocument,
  useFetchDocumentList,
  useRunDocument,
  useSaveDocumentName,
  useSelectRunDocumentLoading,
  useSetDocumentParser,
  useUploadDocument,
  useWebCrawl,
} from '@/hooks/document-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useOneNamespaceEffectsLoading } from '@/hooks/store-hooks';
import { Pagination } from '@/interfaces/common';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { getUnSupportedFilesCount } from '@/utils/document-util';
import { PaginationProps, UploadFile } from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import { KnowledgeRouteKey } from './constant';

export const useFetchDocumentListOnMount = () => {
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const fetchDocumentList = useFetchDocumentList();
  const dispatch = useDispatch();

  useEffect(() => {
    if (knowledgeId) {
      fetchDocumentList();
      dispatch({
        type: 'kFModel/pollGetDocumentList-start',
        payload: knowledgeId,
      });
    }
    return () => {
      dispatch({
        type: 'kFModel/pollGetDocumentList-stop',
      });
    };
  }, [knowledgeId, dispatch, fetchDocumentList]);

  return { fetchDocumentList };
};

export const useGetPagination = (fetchDocumentList: () => void) => {
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

export const useSelectDocumentListLoading = () => {
  return useOneNamespaceEffectsLoading('kFModel', [
    'getKfList',
    'updateDocumentStatus',
  ]);
};

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

export const useHandleSearchChange = (setPagination: () => void) => {
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const throttledGetDocumentList = useCallback(() => {
    dispatch({
      type: 'kFModel/throttledGetDocumentList',
      payload: knowledgeId,
    });
  }, [dispatch, knowledgeId]);

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

export const useRenameDocument = (documentId: string) => {
  const saveName = useSaveDocumentName();

  const {
    visible: renameVisible,
    hideModal: hideRenameModal,
    showModal: showRenameModal,
  } = useSetModalState();
  const loading = useOneNamespaceEffectsLoading('kFModel', ['document_rename']);

  const onRenameOk = useCallback(
    async (name: string) => {
      const ret = await saveName(documentId, name);
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
  const createDocument = useCreateDocument();

  const {
    visible: createVisible,
    hideModal: hideCreateModal,
    showModal: showCreateModal,
  } = useSetModalState();
  const loading = useOneNamespaceEffectsLoading('kFModel', ['document_create']);

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
  const setDocumentParser = useSetDocumentParser();

  const {
    visible: changeParserVisible,
    hideModal: hideChangeParserModal,
    showModal: showChangeParserModal,
  } = useSetModalState();
  const loading = useOneNamespaceEffectsLoading('kFModel', [
    'document_change_parser',
  ]);

  const onChangeParserOk = useCallback(
    async (parserId: string, parserConfig: IChangeParserConfigRequestBody) => {
      const ret = await setDocumentParser(parserId, documentId, parserConfig);
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

  const rowSelection = {
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
  const uploadDocument = useUploadDocument();

  const onDocumentUploadOk = useCallback(
    async (fileList: UploadFile[]): Promise<number | undefined> => {
      if (fileList.length > 0) {
        const ret: any = await uploadDocument(fileList);
        const count = getUnSupportedFilesCount(ret.retmsg);
        /// 500 error code indicates that some file types are not supported
        let retcode = ret.retcode;
        if (
          ret.retcode === 0 ||
          (ret.retcode === 500 && count !== fileList.length) // Some files were not uploaded successfully, but some were uploaded successfully.
        ) {
          retcode = 0;
          hideDocumentUploadModal();
        }
        return retcode;
      }
    },
    [uploadDocument, hideDocumentUploadModal],
  );

  const loading = useOneNamespaceEffectsLoading('kFModel', ['upload_document']);

  return {
    documentUploadLoading: loading,
    onDocumentUploadOk,
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
  };
};

export const useHandleWebCrawl = () => {
  const {
    visible: webCrawlUploadVisible,
    hideModal: hideWebCrawlUploadModal,
    showModal: showWebCrawlUploadModal,
  } = useSetModalState();
  const webCrawl = useWebCrawl();

  const onWebCrawlUploadOk = useCallback(
    async (name: string, url: string) => {
      const ret = await webCrawl(name, url);
      if (ret === 0) {
        hideWebCrawlUploadModal();
        return 0;
      }
      return -1;
    },
    [webCrawl, hideWebCrawlUploadModal],
  );

  const loading = useOneNamespaceEffectsLoading('kFModel', ['web_crawl']);

  return {
    webCrawlUploadLoading: loading,
    onWebCrawlUploadOk,
    webCrawlUploadVisible,
    hideWebCrawlUploadModal,
    showWebCrawlUploadModal,
  };
};

export const useHandleRunDocumentByIds = (id: string) => {
  const loading = useSelectRunDocumentLoading();
  const runDocumentByIds = useRunDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;

  const handleRunDocumentByIds = async (
    documentId: string,
    knowledgeBaseId: string,
    isRunning: boolean,
  ) => {
    if (isLoading) {
      return;
    }
    setCurrentId(documentId);
    try {
      await runDocumentByIds({
        doc_ids: [documentId],
        run: isRunning ? 2 : 1,
        knowledgeBaseId,
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

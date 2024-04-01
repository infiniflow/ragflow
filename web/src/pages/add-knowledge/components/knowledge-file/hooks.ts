import { useSetModalState } from '@/hooks/commonHooks';
import {
  useCreateDocument,
  useFetchDocumentList,
  useSaveDocumentName,
  useSetDocumentParser,
} from '@/hooks/documentHooks';
import { useGetKnowledgeSearchParams } from '@/hooks/routeHook';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useFetchTenantInfo } from '@/hooks/userSettingHook';
import { Pagination } from '@/interfaces/common';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { PaginationProps } from 'antd';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useNavigate, useSelector } from 'umi';
import { KnowledgeRouteKey } from './constant';

export const useFetchDocumentListOnMount = () => {
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const fetchDocumentList = useFetchDocumentList();
  const dispatch = useDispatch();

  useFetchTenantInfo();

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
    };
  }, [kFModel, onPageChange]);

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

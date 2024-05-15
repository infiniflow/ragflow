import { Authorization } from '@/constants/authorization';
import { LanguageTranslationMap } from '@/constants/common';
import { Pagination } from '@/interfaces/common';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import api from '@/utils/api';
import authorizationUtil from '@/utils/authorizationUtil';
import { getSearchValue } from '@/utils/commonUtil';
import { PaginationProps } from 'antd';
import axios from 'axios';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDispatch } from 'umi';
import { useSetModalState, useTranslate } from './commonHooks';
import { useSetDocumentParser } from './documentHooks';
import { useOneNamespaceEffectsLoading } from './storeHooks';
import { useSaveSetting } from './userSettingHook';

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

export const useSetSelectedRecord = <T = IKnowledgeFile>() => {
  const [currentRecord, setCurrentRecord] = useState<T>({} as T);

  const setRecord = (record: T) => {
    setCurrentRecord(record);
  };

  return { currentRecord, setRecord };
};

export const useChangeLanguage = () => {
  const { i18n } = useTranslation();
  const saveSetting = useSaveSetting();

  const changeLanguage = (lng: string) => {
    i18n.changeLanguage(
      LanguageTranslationMap[lng as keyof typeof LanguageTranslationMap],
    );
    saveSetting({ language: lng });
  };

  return changeLanguage;
};

export const useGetPagination = (
  total: number,
  page: number,
  pageSize: number,
  onPageChange: PaginationProps['onChange'],
) => {
  const { t } = useTranslate('common');

  const pagination: PaginationProps = useMemo(() => {
    return {
      showQuickJumper: true,
      total,
      showSizeChanger: true,
      current: page,
      pageSize: pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
      showTotal: (total) => `${t('total')} ${total}`,
    };
  }, [t, onPageChange, page, pageSize, total]);

  return {
    pagination,
  };
};

export const useSetPagination = (namespace: string) => {
  const dispatch = useDispatch();

  const setPagination = useCallback(
    (pageNumber = 1, pageSize?: number) => {
      const pagination: Pagination = {
        current: pageNumber,
      } as Pagination;
      if (pageSize) {
        pagination.pageSize = pageSize;
      }
      dispatch({
        type: `${namespace}/setPagination`,
        payload: pagination,
      });
    },
    [dispatch, namespace],
  );

  return setPagination;
};

export interface AppConf {
  appName: string;
}

export const useFetchAppConf = () => {
  const [appConf, setAppConf] = useState<AppConf>({} as AppConf);
  const fetchAppConf = useCallback(async () => {
    const ret = await axios.get('/conf.json');

    setAppConf(ret.data);
  }, []);

  useEffect(() => {
    fetchAppConf();
  }, [fetchAppConf]);

  return appConf;
};

export const useConnectWithSse = (url: string) => {
  const [content, setContent] = useState<string>('');

  const connect = useCallback(() => {
    const source = new EventSource(
      url || '/sse/createSseEmitter?clientId=123456',
    );

    source.onopen = function () {
      console.log('Connection to the server was opened.');
    };

    source.onmessage = function (event: any) {
      setContent(event.data);
    };

    source.onerror = function (error) {
      console.error('Error occurred:', error);
    };
  }, [url]);

  return { connect, content };
};

export const useConnectWithSseNext = () => {
  const [content, setContent] = useState<string>('');
  const sharedId = getSearchValue('shared_id');
  const authorization = sharedId
    ? 'Bearer ' + sharedId
    : authorizationUtil.getAuthorization();
  const send = useCallback(
    async (body: any) => {
      const response = await fetch(api.completeConversation, {
        method: 'POST',
        headers: {
          [Authorization]: authorization,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
      });
      const reader = response?.body
        ?.pipeThrough(new TextDecoderStream())
        .getReader();

      // const reader = response.body.getReader();

      while (true) {
        const { value, done } = await reader?.read();
        console.log('Received', value);
        setContent(value);
        if (done) break;
      }
      return response;
    },
    [authorization],
  );

  return { send, content };
};

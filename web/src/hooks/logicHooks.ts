import { Authorization } from '@/constants/authorization';
import { LanguageTranslationMap } from '@/constants/common';
import { Pagination } from '@/interfaces/common';
import { ResponseType } from '@/interfaces/database/base';
import { IAnswer } from '@/interfaces/database/chat';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorizationUtil';
import { PaginationProps } from 'antd';
import axios from 'axios';
import { EventSourceParserStream } from 'eventsource-parser/stream';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useDispatch } from 'umi';
import { useSetModalState, useTranslate } from './commonHooks';
import { useSetDocumentParser } from './documentHooks';
import { useFetchLlmList } from './llmHooks';
import { useOneNamespaceEffectsLoading } from './storeHooks';
import {
  useFetchTenantInfo,
  useSaveSetting,
  useSelectTenantInfo,
} from './userSettingHook';

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

export const useSendMessageWithSse = (
  url: string = api.completeConversation,
) => {
  const [answer, setAnswer] = useState<IAnswer>({} as IAnswer);
  const [done, setDone] = useState(true);

  const send = useCallback(
    async (
      body: any,
    ): Promise<{ response: Response; data: ResponseType } | undefined> => {
      try {
        setDone(false);
        const response = await fetch(url, {
          method: 'POST',
          headers: {
            [Authorization]: getAuthorization(),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(body),
        });

        const res = response.clone().json();

        const reader = response?.body
          ?.pipeThrough(new TextDecoderStream())
          .pipeThrough(new EventSourceParserStream())
          .getReader();

        while (true) {
          const x = await reader?.read();
          if (x) {
            const { done, value } = x;
            try {
              const val = JSON.parse(value?.data || '');
              const d = val?.data;
              if (typeof d !== 'boolean') {
                console.info('data:', d);
                setAnswer(d);
              }
            } catch (e) {
              console.warn(e);
            }
            if (done) {
              console.info('done');
              break;
            }
          }
        }
        console.info('done?');
        setDone(true);
        return { data: await res, response };
      } catch (e) {
        setDone(true);
        console.warn(e);
      }
    },
    [url],
  );

  return { send, answer, done };
};

//#region chat hooks

export const useScrollToBottom = (messages?: unknown) => {
  const ref = useRef<HTMLDivElement>(null);

  const scrollToBottom = useCallback(() => {
    if (messages) {
      ref.current?.scrollIntoView({ behavior: 'instant' });
    }
  }, [messages]); // If the message changes, scroll to the bottom

  useEffect(() => {
    scrollToBottom();
  }, [scrollToBottom]);

  return ref;
};

export const useHandleMessageInputChange = () => {
  const [value, setValue] = useState('');

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    const value = e.target.value;
    const nextValue = value.replaceAll('\\n', '\n').replaceAll('\\t', '\t');
    setValue(nextValue);
  };

  return {
    handleInputChange,
    value,
    setValue,
  };
};

// #endregion

/**
 *
 * @param defaultId
 * used to switch between different items, similar to radio
 * @returns
 */
export const useSelectItem = (defaultId?: string) => {
  const [selectedId, setSelectedId] = useState('');

  const handleItemClick = useCallback(
    (id: string) => () => {
      setSelectedId(id);
    },
    [],
  );

  useEffect(() => {
    if (defaultId) {
      setSelectedId(defaultId);
    }
  }, [defaultId]);

  return { selectedId, handleItemClick };
};

export const useFetchModelId = (visible: boolean) => {
  const fetchTenantInfo = useFetchTenantInfo(false);
  const tenantInfo = useSelectTenantInfo();

  useEffect(() => {
    if (visible) {
      fetchTenantInfo();
    }
  }, [visible, fetchTenantInfo]);

  return tenantInfo?.llm_id ?? '';
};

export const useFetchLlmModelOnVisible = (visible: boolean) => {
  const fetchLlmList = useFetchLlmList();

  useEffect(() => {
    if (visible) {
      fetchLlmList();
    }
  }, [fetchLlmList, visible]);
};

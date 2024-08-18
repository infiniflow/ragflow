import { SharedFrom } from '@/constants/chat';
import {
  useCreateNextToken,
  useFetchTokenList,
  useRemoveNextToken,
} from '@/hooks/chat-hooks';
import {
  useSetModalState,
  useShowDeleteConfirm,
  useTranslate,
} from '@/hooks/common-hooks';
import { IStats } from '@/interfaces/database/chat';
import { useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useCallback } from 'react';

export const useOperateApiKey = (dialogId: string, idKey: string) => {
  const { removeToken } = useRemoveNextToken();
  const { createToken, loading: creatingLoading } = useCreateNextToken();
  const { data: tokenList, loading: listLoading } = useFetchTokenList({
    [idKey]: dialogId,
  });

  const showDeleteConfirm = useShowDeleteConfirm();

  const onRemoveToken = (token: string, tenantId: string) => {
    showDeleteConfirm({
      onOk: () => removeToken({ dialogId, tokens: [token], tenantId }),
    });
  };

  const onCreateToken = useCallback(() => {
    createToken({ [idKey]: dialogId });
  }, [createToken, idKey, dialogId]);

  return {
    removeToken: onRemoveToken,
    createToken: onCreateToken,
    tokenList,
    creatingLoading,
    listLoading,
  };
};

type ChartStatsType = {
  [k in keyof IStats]: Array<{ xAxis: string; yAxis: number }>;
};

export const useSelectChartStatsList = (): ChartStatsType => {
  const queryClient = useQueryClient();
  const data = queryClient.getQueriesData({ queryKey: ['fetchStats'] });
  const stats: IStats = data[0][1] as IStats;

  return Object.keys(stats).reduce((pre, cur) => {
    const item = stats[cur as keyof IStats];
    if (item.length > 0) {
      pre[cur as keyof IStats] = item.map((x) => ({
        xAxis: x[0] as string,
        yAxis: x[1] as number,
      }));
    }
    return pre;
  }, {} as ChartStatsType);
};

export const useShowTokenEmptyError = () => {
  const { t } = useTranslate('chat');

  const showTokenEmptyError = useCallback(() => {
    message.error(t('tokenError'));
  }, [t]);
  return { showTokenEmptyError };
};

const getUrlWithToken = (token: string, from: string = 'chat') => {
  const { protocol, host } = window.location;
  return `${protocol}//${host}/chat/share?shared_id=${token}&from=${from}`;
};

const useFetchTokenListBeforeOtherStep = (dialogId: string, idKey: string) => {
  const { showTokenEmptyError } = useShowTokenEmptyError();

  const { data: tokenList, refetch } = useFetchTokenList({ [idKey]: dialogId });

  const token =
    Array.isArray(tokenList) && tokenList.length > 0 ? tokenList[0].token : '';

  const handleOperate = useCallback(async () => {
    const ret = await refetch();
    const list = ret.data;
    if (Array.isArray(list) && list.length > 0) {
      return list[0]?.token;
    } else {
      showTokenEmptyError();
      return false;
    }
  }, [showTokenEmptyError, refetch]);

  return {
    token,
    handleOperate,
  };
};

export const useShowEmbedModal = (dialogId: string, idKey: string) => {
  const {
    visible: embedVisible,
    hideModal: hideEmbedModal,
    showModal: showEmbedModal,
  } = useSetModalState();

  const { handleOperate, token } = useFetchTokenListBeforeOtherStep(
    dialogId,
    idKey,
  );

  const handleShowEmbedModal = useCallback(async () => {
    const succeed = await handleOperate();
    if (succeed) {
      showEmbedModal();
    }
  }, [handleOperate, showEmbedModal]);

  return {
    showEmbedModal: handleShowEmbedModal,
    hideEmbedModal,
    embedVisible,
    embedToken: token,
  };
};

export const usePreviewChat = (dialogId: string, idKey: string) => {
  const { handleOperate } = useFetchTokenListBeforeOtherStep(dialogId, idKey);

  const open = useCallback(
    (t: string) => {
      window.open(
        getUrlWithToken(
          t,
          idKey === 'canvasId' ? SharedFrom.Agent : SharedFrom.Chat,
        ),
        '_blank',
      );
    },
    [idKey],
  );

  const handlePreview = useCallback(async () => {
    const token = await handleOperate();
    if (token) {
      open(token);
    }
  }, [handleOperate, open]);

  return {
    handlePreview,
  };
};

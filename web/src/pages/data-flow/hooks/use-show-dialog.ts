import { useFetchTokenListBeforeOtherStep } from '@/components/embed-dialog/use-show-embed-dialog';
import { SharedFrom } from '@/constants/chat';
import { useShowDeleteConfirm } from '@/hooks/common-hooks';
import {
  useCreateSystemToken,
  useFetchSystemTokenList,
  useRemoveSystemToken,
} from '@/hooks/user-setting-hooks';
import { IStats } from '@/interfaces/database/chat';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';

export const useOperateApiKey = (idKey: string, dialogId?: string) => {
  const { removeToken } = useRemoveSystemToken();
  const { createToken, loading: creatingLoading } = useCreateSystemToken();
  const { data: tokenList, loading: listLoading } = useFetchSystemTokenList();

  const showDeleteConfirm = useShowDeleteConfirm();

  const onRemoveToken = (token: string) => {
    showDeleteConfirm({
      onOk: () => removeToken(token),
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
  const stats: IStats = (data.length > 0 ? data[0][1] : {}) as IStats;

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

const getUrlWithToken = (token: string, from: string = 'chat') => {
  const { protocol, host } = window.location;
  return `${protocol}//${host}/chat/share?shared_id=${token}&from=${from}`;
};

export const usePreviewChat = (idKey: string) => {
  const { handleOperate } = useFetchTokenListBeforeOtherStep();

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

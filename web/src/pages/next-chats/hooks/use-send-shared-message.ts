import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { IEventList } from '@/hooks/use-send-message';
import { useSendAgentMessage } from '@/pages/agent/chat/use-send-agent-message';
import trim from 'lodash/trim';
import { useCallback, useState } from 'react';
import { useSearchParams } from 'umi';

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data_prefix = 'data_';
  const data = Object.fromEntries(
    searchParams
      .entries()
      .filter(([key]) => key.startsWith(data_prefix))
      .map(([key, value]) => [key.replace(data_prefix, ''), value]),
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    locale: searchParams.get('locale'),
    data: data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  };
};

export const useSendNextSharedMessage = (
  addEventList: (data: IEventList, messageId: string) => void,
) => {
  const { from, sharedId: conversationId } = useGetSharedChatSearchParams();
  const url = `/api/v1/${from === SharedFrom.Agent ? 'agentbots' : 'chatbots'}/${conversationId}/completions`;

  const [params, setParams] = useState<any[]>([]);

  const {
    visible: parameterDialogVisible,
    hideModal: hideParameterDialog,
    showModal: showParameterDialog,
  } = useSetModalState();

  const ret = useSendAgentMessage(url, addEventList, params);

  const ok = useCallback(
    (params: any[]) => {
      setParams(params);
      hideParameterDialog();
    },
    [hideParameterDialog],
  );

  return {
    ...ret,
    hasError: false,
    parameterDialogVisible,
    hideParameterDialog,
    showParameterDialog,
    ok,
  };
};

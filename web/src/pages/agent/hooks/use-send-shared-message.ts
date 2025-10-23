import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchExternalAgentInputs } from '@/hooks/use-agent-request';
import { IEventList } from '@/hooks/use-send-message';
import {
  buildRequestBody,
  useSendAgentMessage,
} from '@/pages/agent/chat/use-send-agent-message';
import { isEmpty } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'umi';
import { AgentDialogueMode } from '../constant';

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
  const { data: inputsData } = useFetchExternalAgentInputs();

  const [params, setParams] = useState<any[]>([]);
  const sendedTaskMessage = useRef<boolean>(false);

  const isTaskMode = inputsData.mode === AgentDialogueMode.Task;

  const {
    visible: parameterDialogVisible,
    hideModal: hideParameterDialog,
    showModal: showParameterDialog,
  } = useSetModalState();

  const ret = useSendAgentMessage({
    url,
    addEventList,
    beginParams: params,
    isShared: true,
    isTaskMode,
  });

  const ok = useCallback(
    (params: any[]) => {
      if (isTaskMode) {
        const msgBody = buildRequestBody('');

        ret.sendMessage({
          message: msgBody,
          beginInputs: params,
        });
      } else {
        setParams(params);
      }

      hideParameterDialog();
    },
    [hideParameterDialog, isTaskMode, ret],
  );

  const runTask = useCallback(() => {
    if (
      isTaskMode &&
      isEmpty(inputsData?.inputs) &&
      !sendedTaskMessage.current
    ) {
      ok([]);
      sendedTaskMessage.current = true;
    }
  }, [inputsData?.inputs, isTaskMode, ok]);

  useEffect(() => {
    runTask();
  }, [runTask]);

  return {
    ...ret,
    hasError: false,
    parameterDialogVisible,
    inputsData,
    isTaskMode,
    hideParameterDialog,
    showParameterDialog,
    ok,
  };
};

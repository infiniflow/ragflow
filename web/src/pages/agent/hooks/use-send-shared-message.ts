import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchExternalAgentInputs } from '@/hooks/use-agent-request';
import { IEventList } from '@/hooks/use-send-message';
import {
  buildRequestBody,
  useSendAgentMessage,
} from '@/pages/agent/chat/use-send-agent-message';
import { BeginQuery } from '@/pages/agent/interface';
import { isEmpty } from 'lodash';
import trim from 'lodash/trim';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router';
import { AgentDialogueMode } from '../constant';

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

const DATA_PREFIX = 'data_';

interface SharedChatSearchParams {
  from: SharedFrom;
  sharedId: string | null;
  release: string | null;
  locale: string | null;
  theme: string | null;
  data: Record<string, string>;
  visibleAvatar: boolean;
}

export const useGetSharedChatSearchParams = () => {
  const [searchParams] = useSearchParams();
  const data = Object.fromEntries(
    searchParams
      .entries()
      .filter(([key]) => key.startsWith(DATA_PREFIX))
      .map(([key, value]) => [key.replace(DATA_PREFIX, ''), value]),
  );
  return {
    from: searchParams.get('from') as SharedFrom,
    sharedId: searchParams.get('shared_id'),
    release: searchParams.get('release'),
    locale: searchParams.get('locale'),
    theme: searchParams.get('theme'),
    data,
    visibleAvatar: searchParams.get('visible_avatar')
      ? searchParams.get('visible_avatar') !== '1'
      : true,
  } as SharedChatSearchParams;
};

export const useSendNextSharedMessage = (
  addEventList: (data: IEventList, messageId: string) => void,
) => {
  const {
    from,
    sharedId: conversationId,
    release,
  } = useGetSharedChatSearchParams();
  const botType = from === SharedFrom.Agent ? 'agentbots' : 'chatbots';
  const releaseQuery = release ? `?release=${encodeURIComponent(release)}` : '';
  const url = `/api/v1/${botType}/${conversationId}/completions${releaseQuery}`;
  const { data: inputsData } = useFetchExternalAgentInputs();

  const [params, setParams] = useState<BeginQuery[]>([]);
  const sendedTaskMessage = useRef(false);

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
    releaseMode: release,
  });
  const ok = useCallback(
    (params: BeginQuery[]) => {
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

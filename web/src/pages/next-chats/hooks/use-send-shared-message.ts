import { SharedFrom } from '@/constants/chat';
import { useSendAgentMessage } from '@/pages/agent/chat/use-send-agent-message';
import trim from 'lodash/trim';
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
      .filter(([key, value]) => key.startsWith(data_prefix))
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

export function useSendNextSharedMessage() {
  const { from, sharedId: conversationId } = useGetSharedChatSearchParams();
  const url = `/api/v1/${from === SharedFrom.Agent ? 'agentbots' : 'chatbots'}/${conversationId}/completions`;

  const ret = useSendAgentMessage(url);

  return {
    ...ret,
    hasError: false,
  };
}

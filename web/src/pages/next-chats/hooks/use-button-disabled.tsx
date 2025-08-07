import { useGetChatSearchParams } from '@/hooks/use-chat-request';
import { trim } from 'lodash';
import { useParams } from 'umi';

export const useGetSendButtonDisabled = () => {
  const { conversationId } = useGetChatSearchParams();
  const { id: dialogId } = useParams();

  return dialogId === '' || conversationId === '';
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

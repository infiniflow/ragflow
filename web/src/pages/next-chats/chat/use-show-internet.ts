import { useFetchChat } from '@/hooks/use-chat-request';
import { isEmpty } from 'lodash';

export function useShowInternet() {
  const { data: currentDialog } = useFetchChat();

  return !isEmpty(currentDialog?.prompt_config?.tavily_api_key);
}

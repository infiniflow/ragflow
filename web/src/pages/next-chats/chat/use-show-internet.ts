import { useFetchChat } from '@/hooks/use-chat-request';
import { isEmpty } from 'lodash';
import { getWebSearchApiKey } from './web-search-api-key';

export function useShowInternet() {
  const { data: currentDialog } = useFetchChat();

  return !isEmpty(getWebSearchApiKey(currentDialog?.prompt_config));
}

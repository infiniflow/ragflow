import { useFetchChat } from '@/hooks/use-chat-request';
import { hasWebSearchProvider } from './web-search-api-key';

export function useShowInternet() {
  const { data: currentDialog } = useFetchChat();

  return hasWebSearchProvider(currentDialog?.prompt_config);
}

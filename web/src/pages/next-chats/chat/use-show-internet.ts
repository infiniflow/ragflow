import { useFetchDialog } from '@/hooks/use-chat-request';
import { isEmpty } from 'lodash';

export function useShowInternet() {
  const { data: currentDialog } = useFetchDialog();

  return !isEmpty(currentDialog?.prompt_config?.tavily_api_key);
}

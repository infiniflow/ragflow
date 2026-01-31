import { trim } from 'lodash';
import { useParams } from 'umi';

export const useGetSendButtonDisabled = () => {
  const { id: dialogId } = useParams();

  return dialogId === '';
};

export const useSendButtonDisabled = (value: string) => {
  return trim(value) === '';
};

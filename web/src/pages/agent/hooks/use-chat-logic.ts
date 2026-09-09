import { MessageType } from '@/constants/chat';
import { IMessage, Message } from '@/interfaces/database/chat';
import { get } from 'lodash';
import { useCallback, useMemo } from 'react';
import { BeginQuery } from '../interface';
import { buildBeginQueryWithObject } from '../utils';
type IAwaitCompentData = {
  derivedMessages: IMessage[];
  sendFormMessage: (params: { inputs: Record<string, BeginQuery> }) => void;
};
const useAwaitComponentData = (props: IAwaitCompentData) => {
  const { derivedMessages, sendFormMessage } = props;

  const getInputs = useCallback((message: Message) => {
    return get(message, 'data.inputs', {}) as Record<string, BeginQuery>;
  }, []);

  const buildInputList = useCallback(
    (message: Message) => {
      return Object.entries(getInputs(message)).map(([key, val]) => {
        return {
          ...val,
          key,
        };
      });
    },
    [getInputs],
  );

  const handleOk = useCallback(
    (message: Message) => (values: BeginQuery[]) => {
      const inputs = getInputs(message);
      const nextInputs = buildBeginQueryWithObject(inputs, values);
      sendFormMessage({
        inputs: nextInputs,
      });
    },
    [getInputs, sendFormMessage],
  );

  const isWaiting = useMemo(() => {
    const temp = derivedMessages?.some((message, i) => {
      const hasInputs = Object.keys(getInputs(message)).length > 0;
      const flag =
        message.role === MessageType.Assistant &&
        derivedMessages.length - 1 === i &&
        hasInputs;
      return flag;
    });
    return temp;
  }, [derivedMessages, getInputs]);
  return { getInputs, buildInputList, handleOk, isWaiting };
};

export { useAwaitComponentData };

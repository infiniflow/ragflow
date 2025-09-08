import { MessageType } from '@/constants/chat';
import { Message } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import { get } from 'lodash';
import { useCallback, useMemo } from 'react';
import { BeginQuery } from '../interface';
import { buildBeginQueryWithObject } from '../utils';
type IAwaitCompentData = {
  derivedMessages: IMessage[];
  sendFormMessage: (params: {
    inputs: Record<string, BeginQuery>;
    id: string;
  }) => void;
  canvasId: string;
};
const useAwaitCompentData = (props: IAwaitCompentData) => {
  const { derivedMessages, sendFormMessage, canvasId } = props;

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
        id: canvasId,
      });
    },
    [getInputs, sendFormMessage, canvasId],
  );

  const isWaitting = useMemo(() => {
    const temp = derivedMessages?.some((message, i) => {
      const flag =
        message.role === MessageType.Assistant &&
        derivedMessages.length - 1 === i &&
        message.data;
      return flag;
    });
    return temp;
  }, [derivedMessages]);
  return { getInputs, buildInputList, handleOk, isWaitting };
};

export { useAwaitCompentData };

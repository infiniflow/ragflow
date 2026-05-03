import { MessageType } from '@/constants/chat';
import { IMessage, IReference } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';

export const buildAgentMessageItemReference = (
  conversation: { messages: IMessage[]; reference: IReference[] },
  message: IMessage,
) => {
  const assistantMessages = conversation.messages?.filter(
    (x) => x.role === MessageType.Assistant,
  );
  const referenceIndex = assistantMessages.findIndex(
    (x) => x.id === message.id,
  );
  const reference = !isEmpty(message?.reference)
    ? message?.reference
    : (conversation?.reference ?? [])[referenceIndex];

  return reference ?? { doc_aggs: [], chunks: [], total: 0 };
};

import { MessageType } from '@/constants/chat';
import { IReference } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import { isEmpty } from 'lodash';

export const buildAgentMessageItemReference = (
  conversation: { message: IMessage[]; reference: IReference[] },
  message: IMessage,
) => {
  const assistantMessages = conversation.message?.filter(
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

import { EmptyConversationId, MessageType } from '@/constants/chat';
import { IConversation, IReference } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';
import { IMessage } from '../chat/interface';

export const isConversationIdExist = (conversationId: string) => {
  return conversationId !== EmptyConversationId && conversationId !== '';
};

export const getDocumentIdsFromConversionReference = (data: IConversation) => {
  const documentIds = data.reference.reduce(
    (pre: Array<string>, cur: IReference) => {
      cur.doc_aggs
        ?.map((x) => x.doc_id)
        .forEach((x) => {
          if (pre.every((y) => y !== x)) {
            pre.push(x);
          }
        });
      return pre;
    },
    [],
  );
  return documentIds.join(',');
};

export const buildMessageItemReference = (
  conversation: { message: IMessage[]; reference: IReference[] },
  message: IMessage,
) => {
  const assistantMessages = conversation.message
    ?.filter((x) => x.role === MessageType.Assistant)
    .slice(1);
  const referenceIndex = assistantMessages.findIndex(
    (x) => x.id === message.id,
  );
  const reference = !isEmpty(message?.reference)
    ? message?.reference
    : (conversation?.reference ?? [])[referenceIndex];

  return reference ?? { doc_aggs: [], chunks: [], total: 0 };
};

const oldReg = /(#{2}\d+\${2})/g;
export const currentReg = /\[ID:(\d+)\]/g;

// To be compatible with the old index matching mode
export const replaceTextByOldReg = (text: string) => {
  return text?.replace(oldReg, (substring: string) => {
    return `[ID:${substring.slice(2, -2)}]`;
  });
};

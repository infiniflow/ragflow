import { MessageType } from '@/constants/chat';
import { IConversation, IReference } from '@/interfaces/database/chat';
import { EmptyConversationId } from './constants';
import { IMessage } from './interface';

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
  const reference = message?.reference
    ? message?.reference
    : conversation.reference[referenceIndex];

  return reference;
};

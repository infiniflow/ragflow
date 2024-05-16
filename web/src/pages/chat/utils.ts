import { MessageType } from '@/constants/chat';
import { IConversation, IReference } from '@/interfaces/database/chat';
import { EmptyConversationId, variableEnabledFieldMap } from './constants';
import { IClientConversation, IMessage } from './interface';

export const excludeUnEnabledVariables = (values: any) => {
  const unEnabledFields: Array<keyof typeof variableEnabledFieldMap> =
    Object.keys(variableEnabledFieldMap).filter((key) => !values[key]) as Array<
      keyof typeof variableEnabledFieldMap
    >;

  return unEnabledFields.map(
    (key) => `llm_setting.${variableEnabledFieldMap[key]}`,
  );
};

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
  conversation: IClientConversation,
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

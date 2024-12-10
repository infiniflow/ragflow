import { EmptyConversationId, MessageType } from '@/constants/chat';
import { Message } from '@/interfaces/database/chat';
import { IMessage } from '@/pages/chat/interface';
import { omit } from 'lodash';
import { v4 as uuid } from 'uuid';

export const isConversationIdExist = (conversationId: string) => {
  return conversationId !== EmptyConversationId && conversationId !== '';
};

export const buildMessageUuid = (message: Partial<Message | IMessage>) => {
  if ('id' in message && message.id) {
    return message.role === MessageType.User
      ? `${MessageType.User}_${message.id}`
      : `${MessageType.Assistant}_${message.id}`;
  }
  return uuid();
};

export const getMessagePureId = (id?: string) => {
  const strings = id?.split('_') ?? [];
  if (strings.length > 0) {
    return strings.at(-1);
  }
  return id;
};

export const buildMessageListWithUuid = (messages?: Message[]) => {
  return (
    messages?.map((x: Message | IMessage) => ({
      ...omit(x, 'reference'),
      id: buildMessageUuid(x),
    })) ?? []
  );
};

export const getConversationId = () => {
  return uuid().replace(/-/g, '');
};

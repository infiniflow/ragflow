import { EmptyConversationId } from '@/constants/chat';

export const isConversationIdExist = (conversationId: string) => {
  return conversationId !== EmptyConversationId && conversationId !== '';
};

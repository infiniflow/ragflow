import { createContext } from 'react';

export const ConversationContext = createContext<
  null | ((isPlaying: boolean) => void)
>(null);
